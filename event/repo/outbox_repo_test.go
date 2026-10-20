// Package repo 的发件箱仓储测试
//
// @author zwei
package repo_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/db/dbtest"
	"gitee.com/hq_project/yomi-boot/event/model"
	"gitee.com/hq_project/yomi-boot/event/repo"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// sqlRecorder 记录 GORM 实际执行的 SQL，用于验证批量更新是否分片
type sqlRecorder struct {
	mu    sync.Mutex
	stmts []string
}

// LogMode 返回当前记录器
func (r *sqlRecorder) LogMode(logger.LogLevel) logger.Interface {
	return r
}

// Info 忽略 Info 日志
func (r *sqlRecorder) Info(context.Context, string, ...any) {}

// Warn 忽略 Warn 日志
func (r *sqlRecorder) Warn(context.Context, string, ...any) {}

// Error 忽略 Error 日志
func (r *sqlRecorder) Error(context.Context, string, ...any) {}

// Trace 记录 GORM 执行的 SQL
func (r *sqlRecorder) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stmts = append(r.stmts, sql)
}

// countContains 返回包含指定片段的 SQL 数量
func (r *sqlRecorder) countContains(sub string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, stmt := range r.stmts {
		if strings.Contains(stmt, sub) {
			count++
		}
	}
	return count
}

// TestOutboxRepoUpdateBatchShardsStatements 验证批量全量更新按固定分片执行且数据完整
func TestOutboxRepoUpdateBatchShardsStatements(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(model.SysEventOutbox{}))
	defer cleanup()

	const rowCount = 450
	rows := make([]model.SysEventOutbox, 0, rowCount)
	for i := 1; i <= rowCount; i++ {
		rows = append(rows, model.SysEventOutbox{
			ServiceName: "svc",
			EventID:     fmt.Sprintf("event-%d", i),
			Topic:       "topic",
			Status:      model.OutboxStatusPending,
		})
		rows[len(rows)-1].ID = int64(i)
	}
	if err := gormDB.CreateInBatches(&rows, 100).Error; err != nil {
		t.Fatalf("初始化发件箱数据失败: %v", err)
	}

	recorder := &sqlRecorder{}
	recordedDB := gormDB.Session(&gorm.Session{Logger: recorder})
	r := repo.NewOutboxRepo(recordedDB)

	fetchTime := time.Now()
	for i := range rows {
		rows[i].Status = model.OutboxStatusFetched
		rows[i].FetchTime = &fetchTime
	}
	if err := r.UpdateBatch(context.Background(), rows); err != nil {
		t.Fatalf("批量更新失败: %v", err)
	}

	if got := recorder.countContains("INSERT INTO"); got != 3 {
		t.Fatalf("期望 450 行按 200 行分片产生 3 条插入语句，实际 %d 条", got)
	}

	var stored []model.SysEventOutbox
	if err := gormDB.Where("service_name = ?", "svc").Find(&stored).Error; err != nil {
		t.Fatalf("查询发件箱数据失败: %v", err)
	}
	if len(stored) != rowCount {
		t.Fatalf("期望 %d 行，实际 %d 行", rowCount, len(stored))
	}
	for i := range stored {
		if stored[i].Status != model.OutboxStatusFetched {
			t.Fatalf("第 %d 行状态期望 %s，实际 %s", i, model.OutboxStatusFetched, stored[i].Status)
		}
	}
}

// TestOutboxRepoMarkFetchedBatchOnlyPending 验证只有 PENDING 记录会被标记为 FETCHED
func TestOutboxRepoMarkFetchedBatchOnlyPending(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(model.SysEventOutbox{}))
	defer cleanup()

	rows := []model.SysEventOutbox{
		{ServiceName: "svc", EventID: "event-1", Topic: "topic", Status: model.OutboxStatusPending},
		{ServiceName: "svc", EventID: "event-2", Topic: "topic", Status: model.OutboxStatusPending},
		{ServiceName: "svc", EventID: "event-3", Topic: "topic", Status: model.OutboxStatusSentSuccess},
	}
	for i := range rows {
		rows[i].ID = int64(i + 1)
	}
	if err := gormDB.Create(&rows).Error; err != nil {
		t.Fatalf("初始化发件箱数据失败: %v", err)
	}

	r := repo.NewOutboxRepo(gormDB)
	now := time.Now()
	fetched, err := r.MarkFetchedBatch(context.Background(), []int64{1, 2, 3}, now)
	if err != nil {
		t.Fatalf("标记 FETCHED 失败: %v", err)
	}
	if len(fetched) != 2 {
		t.Fatalf("期望 2 条实际更新，实际 %d 条: %v", len(fetched), fetched)
	}

	var stored []model.SysEventOutbox
	if err := gormDB.Order("id ASC").Find(&stored).Error; err != nil {
		t.Fatalf("查询发件箱数据失败: %v", err)
	}
	if stored[0].Status != model.OutboxStatusFetched || stored[0].FetchTime == nil {
		t.Fatalf("PENDING 记录应变为 FETCHED: %+v", stored[0])
	}
	if stored[1].Status != model.OutboxStatusFetched || stored[1].FetchTime == nil {
		t.Fatalf("PENDING 记录应变为 FETCHED: %+v", stored[1])
	}
	if stored[2].Status != model.OutboxStatusSentSuccess {
		t.Fatalf("SENT_SUCCESS 记录不应被覆盖: %+v", stored[2])
	}
}

// TestOutboxRepoMarkPendingBatchOnlyTimeoutFetched 验证只有超时 FETCHED 记录会被重置
func TestOutboxRepoMarkPendingBatchOnlyTimeoutFetched(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(model.SysEventOutbox{}))
	defer cleanup()

	oldTime := time.Now().Add(-10 * time.Minute)
	freshTime := time.Now()
	rows := []model.SysEventOutbox{
		{ServiceName: "svc", EventID: "event-1", Topic: "topic", Status: model.OutboxStatusFetched, FetchTime: &oldTime},
		{ServiceName: "svc", EventID: "event-2", Topic: "topic", Status: model.OutboxStatusFetched, FetchTime: &freshTime},
		{ServiceName: "svc", EventID: "event-3", Topic: "topic", Status: model.OutboxStatusSentSuccess, FetchTime: &oldTime},
	}
	for i := range rows {
		rows[i].ID = int64(i + 1)
	}
	if err := gormDB.Create(&rows).Error; err != nil {
		t.Fatalf("初始化发件箱数据失败: %v", err)
	}

	r := repo.NewOutboxRepo(gormDB)
	affected, err := r.MarkPendingBatch(context.Background(), []int64{1, 2, 3}, time.Now().Add(-5*time.Minute))
	if err != nil {
		t.Fatalf("重置 PENDING 失败: %v", err)
	}
	if affected != 1 {
		t.Fatalf("期望实际更新 1 条，实际 %d 条", affected)
	}

	var stored []model.SysEventOutbox
	if err := gormDB.Order("id ASC").Find(&stored).Error; err != nil {
		t.Fatalf("查询发件箱数据失败: %v", err)
	}
	if stored[0].Status != model.OutboxStatusPending || stored[0].FetchTime != nil {
		t.Fatalf("超时 FETCHED 记录应重置为 PENDING: %+v", stored[0])
	}
	if stored[1].Status != model.OutboxStatusFetched || stored[1].FetchTime == nil {
		t.Fatalf("未超时 FETCHED 记录不应被重置: %+v", stored[1])
	}
	if stored[2].Status != model.OutboxStatusSentSuccess {
		t.Fatalf("SENT_SUCCESS 记录不应被重置: %+v", stored[2])
	}
}

// TestOutboxRepoMarkSentSuccessOnlyFetched 验证发送成功回写不会覆盖非 FETCHED 状态
func TestOutboxRepoMarkSentSuccessOnlyFetched(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(model.SysEventOutbox{}))
	defer cleanup()

	rows := []model.SysEventOutbox{
		{ServiceName: "svc", EventID: "event-1", Topic: "topic", Status: model.OutboxStatusFetched},
		{ServiceName: "svc", EventID: "event-2", Topic: "topic", Status: model.OutboxStatusSentSuccess},
		{ServiceName: "svc", EventID: "event-3", Topic: "topic", Status: model.OutboxStatusPending},
	}
	for i := range rows {
		rows[i].ID = int64(i + 1)
	}
	if err := gormDB.Create(&rows).Error; err != nil {
		t.Fatalf("初始化发件箱数据失败: %v", err)
	}

	r := repo.NewOutboxRepo(gormDB)
	now := time.Now()
	if updated, err := r.MarkSentSuccess(context.Background(), 1, now); err != nil || !updated {
		t.Fatalf("FETCHED 记录应更新成功: updated=%v err=%v", updated, err)
	}
	if updated, err := r.MarkSentSuccess(context.Background(), 2, now); err != nil || updated {
		t.Fatalf("SENT_SUCCESS 记录不应再次更新: updated=%v err=%v", updated, err)
	}
	if updated, err := r.MarkSentSuccess(context.Background(), 3, now); err != nil || updated {
		t.Fatalf("PENDING 记录不应被覆盖: updated=%v err=%v", updated, err)
	}
}

// TestOutboxRepoMarkSentFailOnlyFetched 验证发送失败回写不会覆盖非 FETCHED 状态
func TestOutboxRepoMarkSentFailOnlyFetched(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(model.SysEventOutbox{}))
	defer cleanup()

	rows := []model.SysEventOutbox{
		{ServiceName: "svc", EventID: "event-1", Topic: "topic", Status: model.OutboxStatusFetched},
		{ServiceName: "svc", EventID: "event-2", Topic: "topic", Status: model.OutboxStatusSentSuccess},
	}
	for i := range rows {
		rows[i].ID = int64(i + 1)
	}
	if err := gormDB.Create(&rows).Error; err != nil {
		t.Fatalf("初始化发件箱数据失败: %v", err)
	}

	r := repo.NewOutboxRepo(gormDB)
	if updated, err := r.MarkSentFail(context.Background(), 1); err != nil || !updated {
		t.Fatalf("FETCHED 记录应更新为 SENT_FAIL: updated=%v err=%v", updated, err)
	}
	if updated, err := r.MarkSentFail(context.Background(), 2); err != nil || updated {
		t.Fatalf("SENT_SUCCESS 记录不应被覆盖: updated=%v err=%v", updated, err)
	}
}

// TestOutboxRepoMarkSentSuccessFromDlq 验证死信成功回写只处理非成功状态
func TestOutboxRepoMarkSentSuccessFromDlq(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(model.SysEventOutbox{}))
	defer cleanup()

	rows := []model.SysEventOutbox{
		{ServiceName: "svc", EventID: "event-1", Topic: "topic", Status: model.OutboxStatusSentFail},
		{ServiceName: "svc", EventID: "event-2", Topic: "topic", Status: model.OutboxStatusSentSuccess},
	}
	for i := range rows {
		rows[i].ID = int64(i + 1)
	}
	if err := gormDB.Create(&rows).Error; err != nil {
		t.Fatalf("初始化发件箱数据失败: %v", err)
	}

	r := repo.NewOutboxRepo(gormDB)
	now := time.Now()
	if updated, err := r.MarkSentSuccessFromDlq(context.Background(), 1, now); err != nil || !updated {
		t.Fatalf("SENT_FAIL 记录应更新成功: updated=%v err=%v", updated, err)
	}
	if updated, err := r.MarkSentSuccessFromDlq(context.Background(), 2, now); err != nil || updated {
		t.Fatalf("SENT_SUCCESS 记录不应再次更新: updated=%v err=%v", updated, err)
	}
}

// TestOutboxRepoRestorePendingBatch 验证批量恢复只重置 FETCHED 记录
func TestOutboxRepoRestorePendingBatch(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(model.SysEventOutbox{}))
	defer cleanup()

	fetchTime := time.Now()
	rows := []model.SysEventOutbox{
		{ServiceName: "svc", EventID: "event-1", Topic: "topic", Status: model.OutboxStatusFetched, FetchTime: &fetchTime},
		{ServiceName: "svc", EventID: "event-2", Topic: "topic", Status: model.OutboxStatusFetched, FetchTime: &fetchTime},
		{ServiceName: "svc", EventID: "event-3", Topic: "topic", Status: model.OutboxStatusSentSuccess},
	}
	for i := range rows {
		rows[i].ID = int64(i + 1)
	}
	if err := gormDB.Create(&rows).Error; err != nil {
		t.Fatalf("初始化发件箱数据失败: %v", err)
	}

	r := repo.NewOutboxRepo(gormDB)
	if err := r.RestorePendingBatch(context.Background(), []int64{1, 2, 3}); err != nil {
		t.Fatalf("批量恢复失败: %v", err)
	}

	var stored []model.SysEventOutbox
	if err := gormDB.Order("id ASC").Find(&stored).Error; err != nil {
		t.Fatalf("查询发件箱数据失败: %v", err)
	}
	if len(stored) != 3 {
		t.Fatalf("期望 3 行，实际 %d 行", len(stored))
	}
	for i := 0; i < 2; i++ {
		if stored[i].Status != model.OutboxStatusPending {
			t.Fatalf("第 %d 行状态期望 %s，实际 %s", i, model.OutboxStatusPending, stored[i].Status)
		}
		if stored[i].FetchTime != nil {
			t.Fatalf("第 %d 行 fetch_time 期望 nil，实际 %v", i, stored[i].FetchTime)
		}
	}
	if stored[2].Status != model.OutboxStatusSentSuccess {
		t.Fatalf("已发送成功记录不应被恢复，实际 %s", stored[2].Status)
	}
}
