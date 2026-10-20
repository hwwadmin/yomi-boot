// Package event 的过期数据清理测试
//
// @author zwei
package event

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/db/dbtest"
	"gitee.com/hq_project/yomi-boot/event/cfg"
	"gitee.com/hq_project/yomi-boot/event/model"
	"gitee.com/hq_project/yomi-boot/event/repo"
)

// TestRetentionCleanerCleanOutbox 验证只删除过期的发送成功且无死信引用的记录
func TestRetentionCleanerCleanOutbox(t *testing.T) {
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

	cleaner := NewRetentionCleaner(
		cfg.Runtime{
			ServiceName: "svc",
			OutboxCleanup: cfg.CleanupRuntime{
				Enabled:    true,
				Retention:  30 * 24 * time.Hour,
				Frequency:  time.Hour,
				BatchSize:  100,
				MaxBatches: 10,
			},
		},
		repo.NewOutboxRepo(gormDB),
		nil,
	)
	if err := cleaner.cleanOutbox(context.Background()); err != nil {
		t.Fatalf("清理发件箱失败: %v", err)
	}

	var count int64
	if err := gormDB.Unscoped().Model(&model.SysEventOutbox{}).Count(&count).Error; err != nil {
		t.Fatalf("查询发件箱数量失败: %v", err)
	}
	if count != 3 {
		t.Fatalf("期望保留 3 条，实际 %d 条", count)
	}
	if err := gormDB.Unscoped().Model(&model.SysEventOutbox{}).Where("id = ?", 1).Count(&count).Error; err != nil {
		t.Fatalf("查询已删除记录失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("过期成功记录应硬删除，实际仍存在 %d 条", count)
	}
}

// TestRetentionCleanerCleanInbox 验证只删除过期且无死信引用的收件箱记录
func TestRetentionCleanerCleanInbox(t *testing.T) {
	gormDB, cleanup := dbtest.New(t,
		dbtest.WithTables(model.SysEventInbox{}, model.SysEventInboxDlq{}),
	)
	defer cleanup()

	oldTime := time.Now().Add(-40 * 24 * time.Hour)
	recentTime := time.Now().Add(-24 * time.Hour)
	rows := []model.SysEventInbox{
		{ServiceName: "svc", EventID: "event-1", Topic: "topic"},
		{ServiceName: "svc", EventID: "event-2", Topic: "topic"},
		{ServiceName: "svc", EventID: "event-3", Topic: "topic"},
	}
	rows[0].CreateTime = oldTime
	rows[1].CreateTime = recentTime
	rows[2].CreateTime = oldTime
	for i := range rows {
		rows[i].ID = int64(i + 1)
	}
	if err := gormDB.Create(&rows).Error; err != nil {
		t.Fatalf("初始化收件箱数据失败: %v", err)
	}
	dlq := &model.SysEventInboxDlq{ServiceName: "svc", EventID: "event-3", Topic: "topic", ErrorMsg: "failed"}
	dlq.ID = 100
	if err := gormDB.Create(dlq).Error; err != nil {
		t.Fatalf("初始化死信数据失败: %v", err)
	}

	cleaner := NewRetentionCleaner(
		cfg.Runtime{
			ServiceName: "svc",
			InboxCleanup: cfg.CleanupRuntime{
				Enabled:    true,
				Retention:  30 * 24 * time.Hour,
				Frequency:  time.Hour,
				BatchSize:  100,
				MaxBatches: 10,
			},
		},
		nil,
		repo.NewInboxRepo(gormDB),
	)
	if err := cleaner.cleanInbox(context.Background()); err != nil {
		t.Fatalf("清理收件箱失败: %v", err)
	}

	var count int64
	if err := gormDB.Unscoped().Model(&model.SysEventInbox{}).Count(&count).Error; err != nil {
		t.Fatalf("查询收件箱数量失败: %v", err)
	}
	if count != 2 {
		t.Fatalf("期望保留 2 条，实际 %d 条", count)
	}
	if err := gormDB.Unscoped().Model(&model.SysEventInbox{}).Where("id = ?", 1).Count(&count).Error; err != nil {
		t.Fatalf("查询已删除记录失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("过期收件箱记录应硬删除，实际仍存在 %d 条", count)
	}
}

// TestRetentionCleanerBatchLimit 验证单轮清理受最大批数限制
func TestRetentionCleanerBatchLimit(t *testing.T) {
	gormDB, cleanup := dbtest.New(t,
		dbtest.WithTables(model.SysEventOutbox{}, model.SysEventOutboxDlq{}),
	)
	defer cleanup()

	oldSent := time.Now().Add(-40 * 24 * time.Hour)
	rows := make([]model.SysEventOutbox, 0, 5)
	for i := 0; i < 5; i++ {
		rows = append(rows, model.SysEventOutbox{
			ServiceName: "svc",
			EventID:     fmt.Sprintf("event-%d", i),
			Topic:       "topic",
			Status:      model.OutboxStatusSentSuccess,
			SentTime:    &oldSent,
		})
		rows[i].ID = int64(i + 1)
	}
	if err := gormDB.Create(&rows).Error; err != nil {
		t.Fatalf("初始化发件箱数据失败: %v", err)
	}

	cleaner := NewRetentionCleaner(
		cfg.Runtime{
			ServiceName: "svc",
			OutboxCleanup: cfg.CleanupRuntime{
				Enabled:    true,
				Retention:  30 * 24 * time.Hour,
				Frequency:  time.Hour,
				BatchSize:  2,
				MaxBatches: 2,
			},
		},
		repo.NewOutboxRepo(gormDB),
		nil,
	)
	if err := cleaner.cleanOutbox(context.Background()); err != nil {
		t.Fatalf("清理发件箱失败: %v", err)
	}

	var count int64
	if err := gormDB.Unscoped().Model(&model.SysEventOutbox{}).Count(&count).Error; err != nil {
		t.Fatalf("查询发件箱数量失败: %v", err)
	}
	if count != 1 {
		t.Fatalf("批数上限 2、批量 2 应删除 4 条，实际剩余 %d 条", count)
	}
}
