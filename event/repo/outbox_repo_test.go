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
