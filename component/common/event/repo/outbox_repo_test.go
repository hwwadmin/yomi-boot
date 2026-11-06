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

	"github.com/hwwadmin/yomi-boot/component/common/db/dbtest"
	"github.com/hwwadmin/yomi-boot/component/common/event/model"
	"github.com/hwwadmin/yomi-boot/component/common/event/repo"

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

// TestOutboxRepoListSentSuccessBefore 验证只返回已过保留期且无死信引用的发送成功记录
func TestOutboxRepoListSentSuccessBefore(t *testing.T) {
	gormDB, cleanup := dbtest.New(t,
		dbtest.WithTables(model.SysEventOutbox{}, model.SysEventOutboxDlq{}),
	)
	defer cleanup()

	oldSent := time.Now().Add(-40 * 24 * time.Hour)
	recentSent := time.Now().Add(-24 * time.Hour)
	rows := []model.SysEventOutbox{
		{ServiceName: "svc", EventID: "event-1", Topic: "topic", Status: model.OutboxStatusSentSuccess, SentTime: &oldSent},
		{ServiceName: "svc", EventID: "event-2", Topic: "topic", Status: model.OutboxStatusSentSuccess, SentTime: &recentSent},
		{ServiceName: "svc", EventID: "event-3", Topic: "topic", Status: model.OutboxStatusSentFail, SentTime: &oldSent},
		{ServiceName: "svc", EventID: "event-4", Topic: "topic", Status: model.OutboxStatusSentSuccess, SentTime: &oldSent},
		{ServiceName: "svc", EventID: "event-5", Topic: "topic", Status: model.OutboxStatusPending},
	}
	for i := range rows {
		rows[i].ID = int64(i + 1)
	}
	if err := gormDB.Create(&rows).Error; err != nil {
		t.Fatalf("初始化发件箱数据失败: %v", err)
	}
	dlq := &model.SysEventOutboxDlq{ServiceName: "svc", EventOutboxID: 4, ErrorMsg: "failed"}
	dlq.ID = 100
	if err := gormDB.Create(dlq).Error; err != nil {
		t.Fatalf("初始化死信数据失败: %v", err)
	}

	r := repo.NewOutboxRepo(gormDB)
	ids, err := r.ListSentSuccessBefore(context.Background(), "svc", time.Now().Add(-30*24*time.Hour), 100)
	if err != nil {
		t.Fatalf("查询可清理发件箱记录失败: %v", err)
	}
	if len(ids) != 1 || ids[0] != 1 {
		t.Fatalf("期望只返回 event-1，实际 %v", ids)
	}
	if got, err := r.ListSentSuccessBefore(context.Background(), "svc", time.Now(), 0); err != nil || got != nil {
		t.Fatalf("limit<=0 应返回空，实际 ids=%v err=%v", got, err)
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

// TestOutboxRepoResetFetchedTimeout 验证超时回收在库内按条件更新，并且单轮不超过上限
func TestOutboxRepoResetFetchedTimeout(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(model.SysEventOutbox{}))
	defer cleanup()

	oldest := time.Now().Add(-30 * time.Minute)
	older := time.Now().Add(-20 * time.Minute)
	newer := time.Now().Add(-10 * time.Minute)
	fresh := time.Now()
	rows := []model.SysEventOutbox{
		{ServiceName: "svc", EventID: "deleted", Topic: "topic", Status: model.OutboxStatusFetched, FetchTime: &oldest},
		{ServiceName: "svc", EventID: "older", Topic: "topic", Status: model.OutboxStatusFetched, FetchTime: &older},
		{ServiceName: "svc", EventID: "newer", Topic: "topic", Status: model.OutboxStatusFetched, FetchTime: &newer},
		{ServiceName: "svc", EventID: "fresh", Topic: "topic", Status: model.OutboxStatusFetched, FetchTime: &fresh},
		{ServiceName: "svc", EventID: "pending", Topic: "topic", Status: model.OutboxStatusPending, FetchTime: &older},
		{ServiceName: "svc", EventID: "sent", Topic: "topic", Status: model.OutboxStatusSentSuccess, FetchTime: &older},
		{ServiceName: "other", EventID: "other", Topic: "topic", Status: model.OutboxStatusFetched, FetchTime: &older},
	}
	for i := range rows {
		rows[i].ID = int64(i + 1)
	}
	if err := gormDB.Create(&rows).Error; err != nil {
		t.Fatalf("初始化发件箱数据失败: %v", err)
	}
	if err := gormDB.Delete(&model.SysEventOutbox{}, rows[0].ID).Error; err != nil {
		t.Fatalf("软删除发件箱记录失败: %v", err)
	}

	r := repo.NewOutboxRepo(gormDB)
	deadline := time.Now().Add(-5 * time.Minute)
	affected, err := r.ResetFetchedTimeout(context.Background(), "svc", deadline, 1)
	if err != nil {
		t.Fatalf("重置超时记录失败: %v", err)
	}
	if affected != 1 {
		t.Fatalf("上限 1 时期望更新 1 条，实际 %d 条", affected)
	}
	assertOutboxStatus(t, gormDB, "older", model.OutboxStatusPending, false)
	assertOutboxStatus(t, gormDB, "newer", model.OutboxStatusFetched, true)
	assertOutboxStatus(t, gormDB, "fresh", model.OutboxStatusFetched, true)
	assertOutboxStatus(t, gormDB, "pending", model.OutboxStatusPending, true)
	assertOutboxStatus(t, gormDB, "sent", model.OutboxStatusSentSuccess, true)
	assertOutboxStatus(t, gormDB, "other", model.OutboxStatusFetched, true)

	var deleted model.SysEventOutbox
	if err := gormDB.Unscoped().First(&deleted, rows[0].ID).Error; err != nil {
		t.Fatalf("查询软删除记录失败: %v", err)
	}
	if deleted.Status != model.OutboxStatusFetched || !deleted.DeleteTime.Valid {
		t.Fatalf("软删除记录不应被重置: %+v", deleted)
	}

	affected, err = r.ResetFetchedTimeout(context.Background(), "svc", deadline, 10)
	if err != nil {
		t.Fatalf("第二轮重置失败: %v", err)
	}
	if affected != 1 {
		t.Fatalf("第二轮期望更新剩余 1 条，实际 %d 条", affected)
	}
	assertOutboxStatus(t, gormDB, "newer", model.OutboxStatusPending, false)
	assertOutboxStatus(t, gormDB, "fresh", model.OutboxStatusFetched, true)

	if _, err := r.ResetFetchedTimeout(context.Background(), "svc", deadline, 0); err == nil {
		t.Fatal("上限不大于 0 应返回错误")
	}
	assertOutboxStatus(t, gormDB, "fresh", model.OutboxStatusFetched, true)
}

// assertOutboxStatus 按 eventId 校验状态，以及是否仍保留取走时间
func assertOutboxStatus(t *testing.T, gormDB *gorm.DB, eventID string, status model.OutboxStatus, hasFetchTime bool) {
	t.Helper()
	var stored model.SysEventOutbox
	if err := gormDB.Where("event_id = ?", eventID).First(&stored).Error; err != nil {
		t.Fatalf("查询 %s 失败: %v", eventID, err)
	}
	if stored.Status != status {
		t.Fatalf("%s 状态期望 %s，实际 %+v", eventID, status, stored)
	}
	if hasFetchTime != (stored.FetchTime != nil) {
		t.Fatalf("%s 取走时间期望保留=%v，实际 %+v", eventID, hasFetchTime, stored)
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

// TestOutboxRepoInsertAndFindByIDs 验证插入回填主键与按主键批量查询
func TestOutboxRepoInsertAndFindByIDs(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(model.SysEventOutbox{}))
	defer cleanup()

	r := repo.NewOutboxRepo(gormDB)
	rows := []model.SysEventOutbox{
		{ServiceName: "svc", EventID: "event-1", Topic: "topic", Status: model.OutboxStatusPending},
		{ServiceName: "svc", EventID: "event-2", Topic: "topic", Status: model.OutboxStatusPending},
	}
	for i := range rows {
		if err := r.Insert(context.Background(), &rows[i]); err != nil {
			t.Fatalf("插入发件箱记录失败: %v", err)
		}
	}
	if rows[0].ID == 0 || rows[1].ID == 0 {
		t.Fatalf("插入后主键应回填: %d %d", rows[0].ID, rows[1].ID)
	}

	stored, err := r.FindByIDs(context.Background(), []int64{rows[0].ID, rows[1].ID, 9999})
	if err != nil {
		t.Fatalf("按主键查询失败: %v", err)
	}
	if len(stored) != 2 {
		t.Fatalf("期望查回 2 条，实际 %d 条", len(stored))
	}
	if stored[0].EventID != "event-1" || stored[1].EventID != "event-2" {
		t.Fatalf("返回记录与主键不符: %s %s", stored[0].EventID, stored[1].EventID)
	}

	empty, err := r.FindByIDs(context.Background(), nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("空 ID 列表应返回空且不报错: rows=%v err=%v", empty, err)
	}
}

// TestOutboxRepoFetchPending 验证只返回本服务 PENDING 事件且按创建时间升序限量
func TestOutboxRepoFetchPending(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(model.SysEventOutbox{}))
	defer cleanup()

	base := time.Now().Add(-time.Hour)
	rows := []model.SysEventOutbox{
		{ServiceName: "svc", EventID: "event-1", Topic: "topic", Status: model.OutboxStatusPending},
		{ServiceName: "svc", EventID: "event-2", Topic: "topic", Status: model.OutboxStatusFetched},
		{ServiceName: "svc", EventID: "event-3", Topic: "topic", Status: model.OutboxStatusPending},
		{ServiceName: "other", EventID: "event-4", Topic: "topic", Status: model.OutboxStatusPending},
	}
	createTimes := []time.Time{
		base,
		base.Add(-3 * time.Minute),
		base.Add(-2 * time.Minute),
		base.Add(-4 * time.Minute),
	}
	for i := range rows {
		rows[i].ID = int64(i + 1)
		rows[i].CreateTime = createTimes[i]
	}
	if err := gormDB.Create(&rows).Error; err != nil {
		t.Fatalf("初始化发件箱数据失败: %v", err)
	}

	r := repo.NewOutboxRepo(gormDB)
	got, err := r.FetchPending(context.Background(), "svc", 10)
	if err != nil {
		t.Fatalf("查询待处理事件失败: %v", err)
	}
	if len(got) != 2 || got[0].EventID != "event-3" || got[1].EventID != "event-1" {
		t.Fatalf("期望按创建时间升序返回 event-3/event-1，实际 %v", outboxEventIDs(got))
	}

	got, err = r.FetchPending(context.Background(), "svc", 1)
	if err != nil {
		t.Fatalf("限量查询待处理事件失败: %v", err)
	}
	if len(got) != 1 || got[0].EventID != "event-3" {
		t.Fatalf("限量 1 条期望只返回 event-3，实际 %v", outboxEventIDs(got))
	}
}

// TestOutboxRepoUpdate 验证全量更新按主键覆盖全部字段
func TestOutboxRepoUpdate(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(model.SysEventOutbox{}))
	defer cleanup()

	r := repo.NewOutboxRepo(gormDB)
	row := model.SysEventOutbox{ServiceName: "svc", EventID: "event-1", Topic: "topic", Status: model.OutboxStatusPending}
	if err := r.Insert(context.Background(), &row); err != nil {
		t.Fatalf("插入发件箱记录失败: %v", err)
	}

	fetchTime := time.Now().Add(-time.Minute)
	sentTime := time.Now()
	row.Topic = "topic-2"
	row.Payload = "payload"
	row.Status = model.OutboxStatusSentFail
	row.FetchTime = &fetchTime
	row.SentTime = &sentTime
	if err := r.Update(context.Background(), &row); err != nil {
		t.Fatalf("全量更新失败: %v", err)
	}

	var stored model.SysEventOutbox
	if err := gormDB.First(&stored, row.ID).Error; err != nil {
		t.Fatalf("查询发件箱记录失败: %v", err)
	}
	if stored.Topic != "topic-2" || stored.Payload != "payload" || stored.Status != model.OutboxStatusSentFail {
		t.Fatalf("全量更新字段未生效: %+v", stored)
	}
	if stored.FetchTime == nil || stored.SentTime == nil {
		t.Fatalf("全量更新应覆盖时间字段: %+v", stored)
	}
}

// outboxEventIDs 提取发件箱记录的 EventID 列表，便于断言输出
func outboxEventIDs(rows []model.SysEventOutbox) []string {
	ids := make([]string, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].EventID)
	}
	return ids
}
