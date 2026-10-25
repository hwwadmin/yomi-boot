// Package event 的过期数据清理测试
//
// @author zwei
package event

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/db/dbtest"
	"gitee.com/hq_project/yomi-boot/component/common/event/cfg"
	"gitee.com/hq_project/yomi-boot/component/common/event/model"
	"gitee.com/hq_project/yomi-boot/component/common/event/repo"
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

// TestValidateCleanupRuntime 验证清理运行时配置校验
func TestValidateCleanupRuntime(t *testing.T) {
	valid := cfg.CleanupRuntime{
		Enabled:    true,
		Retention:  30 * 24 * time.Hour,
		Frequency:  time.Hour,
		BatchSize:  100,
		MaxBatches: 10,
	}
	if err := validateCleanupRuntime("outbox", valid); err != nil {
		t.Fatalf("合法配置不应报错: %v", err)
	}

	cases := []struct {
		name  string
		mutte func(*cfg.CleanupRuntime)
	}{
		{name: "retention 为 0", mutte: func(c *cfg.CleanupRuntime) { c.Retention = 0 }},
		{name: "frequency 过小", mutte: func(c *cfg.CleanupRuntime) { c.Frequency = time.Millisecond }},
		{name: "batch_size 为 0", mutte: func(c *cfg.CleanupRuntime) { c.BatchSize = 0 }},
		{name: "max_batches 为 0", mutte: func(c *cfg.CleanupRuntime) { c.MaxBatches = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			invalid := valid
			tc.mutte(&invalid)
			if err := validateCleanupRuntime("outbox", invalid); err == nil {
				t.Fatalf("%s 应返回错误", tc.name)
			}
		})
	}
}

// TestCleanupRetentionDays 验证保留时长换算天数仅用于日志
func TestCleanupRetentionDays(t *testing.T) {
	cases := []struct {
		retention time.Duration
		want      int
	}{
		{retention: 0, want: 0},
		{retention: -24 * time.Hour, want: 0},
		{retention: 30 * 24 * time.Hour, want: 30},
		{retention: 36 * time.Hour, want: 1},
	}
	for _, tc := range cases {
		if got := cleanupRetentionDays(tc.retention); got != tc.want {
			t.Fatalf("retention=%s 期望 %d 天，实际 %d", tc.retention, tc.want, got)
		}
	}
}

// TestRetentionCleanerStartValidation 验证启动时的依赖与配置校验
func TestRetentionCleanerStartValidation(t *testing.T) {
	ctx := context.Background()
	enabled := cfg.CleanupRuntime{
		Enabled:    true,
		Retention:  30 * 24 * time.Hour,
		Frequency:  time.Hour,
		BatchSize:  100,
		MaxBatches: 10,
	}

	t.Run("nil 接收器直接返回", func(t *testing.T) {
		var cleaner *RetentionCleaner
		if err := cleaner.Start(ctx, nil); err != nil {
			t.Fatalf("nil 清理器应直接返回 nil，实际 %v", err)
		}
	})

	t.Run("缺少分布式锁工厂", func(t *testing.T) {
		cleaner := NewRetentionCleaner(cfg.Runtime{ServiceName: "svc"}, nil, nil)
		if err := cleaner.Start(ctx, nil); err == nil {
			t.Fatal("nil 分布式锁工厂应返回错误")
		}
	})

	t.Run("未启用时跳过", func(t *testing.T) {
		cleaner := NewRetentionCleaner(cfg.Runtime{ServiceName: "svc"}, nil, nil)
		if err := cleaner.Start(ctx, nil); err == nil {
			t.Fatal("nil 工厂应先于启用判断报错")
		}
	})

	t.Run("发布侧缺仓储", func(t *testing.T) {
		cleaner := NewRetentionCleaner(
			cfg.Runtime{ServiceName: "svc", OutboxCleanup: enabled}, nil, nil,
		)
		if err := cleaner.Start(ctx, &stubDistLockFactory{}); err == nil {
			t.Fatal("发件箱仓储缺失应返回错误")
		}
	})

	t.Run("消费侧缺仓储", func(t *testing.T) {
		cleaner := NewRetentionCleaner(
			cfg.Runtime{ServiceName: "svc", InboxCleanup: enabled}, nil, nil,
		)
		if err := cleaner.Start(ctx, &stubDistLockFactory{}); err == nil {
			t.Fatal("收件箱仓储缺失应返回错误")
		}
	})

	t.Run("清理配置非法", func(t *testing.T) {
		invalid := enabled
		invalid.Retention = 0
		cleaner := NewRetentionCleaner(
			cfg.Runtime{ServiceName: "svc", OutboxCleanup: invalid}, nil, nil,
		)
		if err := cleaner.Start(ctx, &stubDistLockFactory{}); err == nil {
			t.Fatal("清理配置非法应返回错误")
		}
	})
}

// TestRetentionCleanerCleanError 验证仓储故障时清理任务错误透传
func TestRetentionCleanerCleanError(t *testing.T) {
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
		repo.NewOutboxRepo(newDeadDB(t)),
		nil,
	)
	if err := cleaner.cleanOutbox(context.Background()); err == nil {
		t.Fatal("数据库不可用时清理应返回错误")
	}
}
