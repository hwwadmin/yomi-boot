// Package outbox 的死信错误信息测试
//
// @author zwei
package outbox

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"gitee.com/hq_project/yomi-boot/component/common/db/tx"
	"gitee.com/hq_project/yomi-boot/component/common/event/cfg"
	"gitee.com/hq_project/yomi-boot/component/common/event/model"
	"gitee.com/hq_project/yomi-boot/component/common/event/repo"
	"gitee.com/hq_project/yomi-boot/component/common/snowflake/snowflaketest"

	yomikafka "gitee.com/hq_project/yomi-boot/component/common/kafka"
	"gorm.io/gorm"
)

// TestCoreErrorMessage 验证死信错误信息按字符截断且不破坏 UTF-8
func TestCoreErrorMessage(t *testing.T) {
	if got := coreErrorMessage(nil); got != "" {
		t.Fatalf("nil error 期望空串，实际 %q", got)
	}

	short := errors.New("boom")
	if got := coreErrorMessage(short); got != "boom" {
		t.Fatalf("短错误期望原样返回，实际 %q", got)
	}

	exact := errors.New(strings.Repeat("错", dlqErrorMessageLimit))
	if got := coreErrorMessage(exact); got != exact.Error() {
		t.Fatalf("未超限错误不应截断，实际长度 %d", utf8.RuneCountInString(got))
	}

	long := errors.New(strings.Repeat("错", dlqErrorMessageLimit+100))
	got := coreErrorMessage(long)
	if !utf8.ValidString(got) {
		t.Fatal("截断后的错误信息不是合法 UTF-8")
	}
	if !strings.HasSuffix(got, dlqErrorMessageSuffix) {
		t.Fatalf("截断后的错误信息缺少截断标记: %q", got)
	}
	wantLen := dlqErrorMessageLimit + utf8.RuneCountInString(dlqErrorMessageSuffix)
	if utf8.RuneCountInString(got) != wantLen {
		t.Fatalf("截断长度期望 %d，实际 %d", wantLen, utf8.RuneCountInString(got))
	}
}

// newTestDlqHandler 构造带真实仓储的死信处理器
func newTestDlqHandler(
	t *testing.T,
	publisher yomikafka.PublisherService,
	executor tx.Executor,
) (*DlqHandler, *gorm.DB, *repo.OutboxRepo, *repo.OutboxDlqRepo, func()) {
	t.Helper()
	gormDB, outboxRepo, dlqRepo, cleanup := newOutboxTestDB(t)
	runtime := cfg.Runtime{
		ServiceName:        "svc",
		OutboxDlqFrequency: time.Second,
		OutboxDlqSize:      10,
		OutboxDlqMaxRetry:  3,
	}
	h := NewDlqHandler(runtime, outboxRepo, dlqRepo, publisher, executor, snowflaketest.NewTestGenerator())
	return h, gormDB, outboxRepo, dlqRepo, cleanup
}

// mustInsertOutboxRow 写入一条显式主键的发件箱记录
func mustInsertOutboxRow(t *testing.T, gormDB *gorm.DB, row model.SysEventOutbox) {
	t.Helper()
	if err := gormDB.Create(&row).Error; err != nil {
		t.Fatalf("初始化发件箱数据失败: %v", err)
	}
}

// mustInsertDlqRow 写入一条显式主键的死信记录
func mustInsertDlqRow(t *testing.T, gormDB *gorm.DB, row model.SysEventOutboxDlq) {
	t.Helper()
	if err := gormDB.Create(&row).Error; err != nil {
		t.Fatalf("初始化死信数据失败: %v", err)
	}
}

// fetchDueDlq 取本服务第一条到期死信记录
func fetchDueDlq(t *testing.T, dlqRepo *repo.OutboxDlqRepo) model.SysEventOutboxDlq {
	t.Helper()
	entries, err := dlqRepo.Fetch(context.Background(), "svc", 10, 3, time.Now())
	if err != nil {
		t.Fatalf("查询到期死信失败: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("期望存在到期死信记录")
	}
	return entries[0]
}

// countDlqRows 统计死信记录数
func countDlqRows(t *testing.T, gormDB *gorm.DB) int64 {
	t.Helper()
	var count int64
	if err := gormDB.Model(&model.SysEventOutboxDlq{}).Count(&count).Error; err != nil {
		t.Fatalf("统计死信记录失败: %v", err)
	}
	return count
}

// TestDlqHandlerEnqueue 验证失败事件写入 DLQ 及 ID 生成失败时的错误传递
func TestDlqHandlerEnqueue(t *testing.T) {
	h, gormDB, _, dlqRepo, cleanup := newTestDlqHandler(t, &stubPublisher{}, passthroughExecutor{})
	defer cleanup()

	row := model.SysEventOutbox{ServiceName: "svc", EventID: "event-1", Topic: "topic"}
	row.ID = 7
	if err := h.Enqueue(context.Background(), row, errors.New("boom")); err != nil {
		t.Fatalf("写入死信失败: %v", err)
	}
	var got []model.SysEventOutboxDlq
	if err := gormDB.Find(&got).Error; err != nil {
		t.Fatalf("查询死信失败: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("期望 1 条死信，实际 %d 条", len(got))
	}
	if got[0].ServiceName != "svc" || got[0].EventOutboxID != 7 || got[0].ErrorMsg != "boom" {
		t.Fatalf("死信字段不符: %+v", got[0])
	}
	if got[0].ID == 0 {
		t.Fatal("死信主键应由雪花 ID 生成")
	}

	failHandler := NewDlqHandler(
		cfg.Runtime{ServiceName: "svc"}, nil, dlqRepo, &stubPublisher{},
		passthroughExecutor{}, errIDGen{err: errors.New("no id")},
	)
	if err := failHandler.Enqueue(context.Background(), row, errors.New("boom")); err == nil {
		t.Fatal("ID 生成失败应返回错误")
	}
}

// TestDlqHandlerResetRetryCountInTimeRange 验证按服务过滤的重试计数重置
func TestDlqHandlerResetRetryCountInTimeRange(t *testing.T) {
	h, gormDB, _, _, cleanup := newTestDlqHandler(t, &stubPublisher{}, passthroughExecutor{})
	defer cleanup()

	base := time.Now().Add(-time.Hour)
	mustInsertDlqRow(t, gormDB, model.SysEventOutboxDlq{ServiceName: "svc", EventOutboxID: 1, RetryCount: 2})
	mustInsertDlqRow(t, gormDB, model.SysEventOutboxDlq{ServiceName: "other", EventOutboxID: 2, RetryCount: 5})

	affected, err := h.ResetRetryCountInTimeRange(context.Background(), base, time.Now())
	if err != nil {
		t.Fatalf("重置重试计数失败: %v", err)
	}
	if affected != 1 {
		t.Fatalf("期望重置 1 条，实际 %d 条", affected)
	}
	var got []model.SysEventOutboxDlq
	if err := gormDB.Order("id ASC").Find(&got).Error; err != nil {
		t.Fatalf("查询死信失败: %v", err)
	}
	if got[0].RetryCount != 0 {
		t.Fatalf("本服务死信计数应清零，实际 %d", got[0].RetryCount)
	}
	if got[1].RetryCount != 5 {
		t.Fatalf("其他服务死信不应被重置，实际 %d", got[1].RetryCount)
	}
}

// TestDlqHandlerProcessOne 覆盖死信单条处理的各种分支
func TestDlqHandlerProcessOne(t *testing.T) {
	ctx := context.Background()

	t.Run("孤立死信清理", func(t *testing.T) {
		h, gormDB, _, dlqRepo, cleanup := newTestDlqHandler(t, &stubPublisher{}, passthroughExecutor{})
		defer cleanup()
		mustInsertDlqRow(t, gormDB, model.SysEventOutboxDlq{ServiceName: "svc", EventOutboxID: 99, ErrorMsg: "orphan"})

		dlq := fetchDueDlq(t, dlqRepo)
		h.processOne(ctx, dlq, model.SysEventOutbox{})
		if got := countDlqRows(t, gormDB); got != 0 {
			t.Fatalf("孤立死信应被删除，实际 %d 条", got)
		}
	})

	t.Run("已成功事件清理死信", func(t *testing.T) {
		pub := &stubPublisher{}
		h, gormDB, _, dlqRepo, cleanup := newTestDlqHandler(t, pub, passthroughExecutor{})
		defer cleanup()
		outbox := model.SysEventOutbox{ServiceName: "svc", EventID: "event-1", Topic: "topic", Status: model.OutboxStatusSentSuccess}
		outbox.ID = 1
		mustInsertOutboxRow(t, gormDB, outbox)
		mustInsertDlqRow(t, gormDB, model.SysEventOutboxDlq{ServiceName: "svc", EventOutboxID: 1, ErrorMsg: "stale"})

		dlq := fetchDueDlq(t, dlqRepo)
		h.processOne(ctx, dlq, outbox)
		if got := countDlqRows(t, gormDB); got != 0 {
			t.Fatalf("已成功事件的死信应被删除，实际 %d 条", got)
		}
		if pub.calls() != 0 {
			t.Fatalf("已成功事件不应重试推送，实际推送 %d 次", pub.calls())
		}
	})

	t.Run("重试失败更新计数", func(t *testing.T) {
		pub := &stubPublisher{err: errors.New("boom")}
		h, gormDB, _, dlqRepo, cleanup := newTestDlqHandler(t, pub, passthroughExecutor{})
		defer cleanup()
		outbox := model.SysEventOutbox{ServiceName: "svc", EventID: "event-1", Topic: "topic", Status: model.OutboxStatusSentFail}
		outbox.ID = 1
		mustInsertOutboxRow(t, gormDB, outbox)
		mustInsertDlqRow(t, gormDB, model.SysEventOutboxDlq{ServiceName: "svc", EventOutboxID: 1, ErrorMsg: "boom"})

		dlq := fetchDueDlq(t, dlqRepo)
		h.processOne(ctx, dlq, outbox)

		var got model.SysEventOutboxDlq
		if err := gormDB.First(&got, dlq.ID).Error; err != nil {
			t.Fatalf("查询死信失败: %v", err)
		}
		if got.RetryCount != 1 {
			t.Fatalf("重试次数期望 1，实际 %d", got.RetryCount)
		}
		if got.ErrorMsg != "boom" {
			t.Fatalf("未耗尽重试不应追加耗尽标记，实际 %q", got.ErrorMsg)
		}
		if got.RetryLastTime == nil || got.RetryAfterTime == nil {
			t.Fatalf("重试失败应记录最后重试时间与下次可重投时间: %+v", got)
		}
	})

	t.Run("重试耗尽标记", func(t *testing.T) {
		pub := &stubPublisher{err: errors.New("boom")}
		h, gormDB, _, dlqRepo, cleanup := newTestDlqHandler(t, pub, passthroughExecutor{})
		defer cleanup()
		outbox := model.SysEventOutbox{ServiceName: "svc", EventID: "event-1", Topic: "topic", Status: model.OutboxStatusSentFail}
		outbox.ID = 1
		mustInsertOutboxRow(t, gormDB, outbox)
		mustInsertDlqRow(t, gormDB, model.SysEventOutboxDlq{ServiceName: "svc", EventOutboxID: 1, ErrorMsg: "boom", RetryCount: 2})

		dlq := fetchDueDlq(t, dlqRepo)
		h.processOne(ctx, dlq, outbox)

		var got model.SysEventOutboxDlq
		if err := gormDB.First(&got, dlq.ID).Error; err != nil {
			t.Fatalf("查询死信失败: %v", err)
		}
		if got.RetryCount != 3 {
			t.Fatalf("重试次数期望 3，实际 %d", got.RetryCount)
		}
		if !strings.HasPrefix(got.ErrorMsg, dlqExhaustedErrorPrefix) {
			t.Fatalf("耗尽后错误信息应带 %q 前缀，实际 %q", dlqExhaustedErrorPrefix, got.ErrorMsg)
		}
	})

	t.Run("超过最大重试跳过", func(t *testing.T) {
		pub := &stubPublisher{err: errors.New("boom")}
		h, _, _, _, cleanup := newTestDlqHandler(t, pub, passthroughExecutor{})
		defer cleanup()
		outbox := model.SysEventOutbox{ServiceName: "svc", EventID: "event-1", Topic: "topic", Status: model.OutboxStatusSentFail}
		outbox.ID = 1
		dlq := model.SysEventOutboxDlq{ServiceName: "svc", EventOutboxID: 1, ErrorMsg: "boom", RetryCount: 3}
		dlq.ID = 5

		h.processOne(ctx, dlq, outbox)
		if pub.calls() != 0 {
			t.Fatalf("超过最大重试次数不应推送，实际推送 %d 次", pub.calls())
		}
	})

	t.Run("重投成功回写", func(t *testing.T) {
		pub := &stubPublisher{}
		h, gormDB, _, dlqRepo, cleanup := newTestDlqHandler(t, pub, passthroughExecutor{})
		defer cleanup()
		outbox := model.SysEventOutbox{ServiceName: "svc", EventID: "event-1", Topic: "topic", Status: model.OutboxStatusSentFail}
		outbox.ID = 1
		mustInsertOutboxRow(t, gormDB, outbox)
		mustInsertDlqRow(t, gormDB, model.SysEventOutboxDlq{ServiceName: "svc", EventOutboxID: 1, ErrorMsg: "boom"})

		dlq := fetchDueDlq(t, dlqRepo)
		h.processOne(ctx, dlq, outbox)

		if got := countDlqRows(t, gormDB); got != 0 {
			t.Fatalf("重投成功的死信应被删除，实际 %d 条", got)
		}
		var stored model.SysEventOutbox
		if err := gormDB.First(&stored, outbox.ID).Error; err != nil {
			t.Fatalf("查询发件箱失败: %v", err)
		}
		if stored.Status != model.OutboxStatusSentSuccess || stored.SentTime == nil {
			t.Fatalf("发件箱应回写为发送成功: %+v", stored)
		}
	})
}

// TestDlqHandlerProcessDlq 验证从拉取到分发的整体流程
func TestDlqHandlerProcessDlq(t *testing.T) {
	h, gormDB, _, _, cleanup := newTestDlqHandler(t, &stubPublisher{}, passthroughExecutor{})
	defer cleanup()

	outbox1 := model.SysEventOutbox{
		ServiceName: "svc", EventID: "event-1", Topic: "topic", Status: model.OutboxStatusSentFail,
	}
	outbox1.ID = 1
	mustInsertOutboxRow(t, gormDB, outbox1)
	outbox2 := model.SysEventOutbox{
		ServiceName: "svc", EventID: "event-2", Topic: "topic", Status: model.OutboxStatusSentSuccess,
	}
	outbox2.ID = 2
	mustInsertOutboxRow(t, gormDB, outbox2)
	mustInsertDlqRow(t, gormDB, model.SysEventOutboxDlq{ServiceName: "svc", EventOutboxID: 1, ErrorMsg: "e1"})
	mustInsertDlqRow(t, gormDB, model.SysEventOutboxDlq{ServiceName: "svc", EventOutboxID: 2, ErrorMsg: "e2"})
	mustInsertDlqRow(t, gormDB, model.SysEventOutboxDlq{ServiceName: "svc", EventOutboxID: 99, ErrorMsg: "e3"})

	if err := h.processDlq(context.Background()); err != nil {
		t.Fatalf("处理死信失败: %v", err)
	}

	if got := countDlqRows(t, gormDB); got != 0 {
		t.Fatalf("处理后死信应清空，实际 %d 条", got)
	}
	var stored []model.SysEventOutbox
	if err := gormDB.Order("id ASC").Find(&stored).Error; err != nil {
		t.Fatalf("查询发件箱失败: %v", err)
	}
	for i := range stored {
		if stored[i].Status != model.OutboxStatusSentSuccess {
			t.Fatalf("发件箱 %d 期望发送成功，实际 %s", stored[i].ID, stored[i].Status)
		}
	}
}

// TestDlqHandlerMarkSentSuccessAndClearDlqWithRetry 验证回写补偿的成功与重试耗尽
func TestDlqHandlerMarkSentSuccessAndClearDlqWithRetry(t *testing.T) {
	ctx := context.Background()

	t.Run("成功", func(t *testing.T) {
		h, gormDB, _, dlqRepo, cleanup := newTestDlqHandler(t, &stubPublisher{}, passthroughExecutor{})
		defer cleanup()
		outbox := model.SysEventOutbox{ServiceName: "svc", EventID: "event-1", Topic: "topic", Status: model.OutboxStatusSentFail}
		outbox.ID = 1
		mustInsertOutboxRow(t, gormDB, outbox)
		mustInsertDlqRow(t, gormDB, model.SysEventOutboxDlq{ServiceName: "svc", EventOutboxID: 1, ErrorMsg: "boom"})
		dlq := fetchDueDlq(t, dlqRepo)

		if !h.markSentSuccessAndClearDlqWithRetry(ctx, &outbox, &dlq, time.Now()) {
			t.Fatal("回写应成功")
		}
		if got := countDlqRows(t, gormDB); got != 0 {
			t.Fatalf("死信应被删除，实际 %d 条", got)
		}
	})

	t.Run("重试耗尽", func(t *testing.T) {
		h, gormDB, _, dlqRepo, cleanup := newTestDlqHandler(
			t, &stubPublisher{}, failExecutor{err: errors.New("db down")},
		)
		defer cleanup()
		outbox := model.SysEventOutbox{ServiceName: "svc", EventID: "event-1", Topic: "topic", Status: model.OutboxStatusSentFail}
		outbox.ID = 1
		mustInsertOutboxRow(t, gormDB, outbox)
		mustInsertDlqRow(t, gormDB, model.SysEventOutboxDlq{ServiceName: "svc", EventOutboxID: 1, ErrorMsg: "boom"})
		dlq := fetchDueDlq(t, dlqRepo)

		if h.markSentSuccessAndClearDlqWithRetry(ctx, &outbox, &dlq, time.Now()) {
			t.Fatal("事务持续失败时回写应返回 false")
		}
		if got := countDlqRows(t, gormDB); got != 1 {
			t.Fatalf("回写失败死信应保留，实际 %d 条", got)
		}
	})
}

// TestDlqExhaustedErrorMsg 验证耗尽标记前缀的拼接与幂等
func TestDlqExhaustedErrorMsg(t *testing.T) {
	if got := dlqExhaustedErrorMsg(""); got != dlqExhaustedErrorPrefix+"已达最大重试次数" {
		t.Fatalf("空错误信息期望仅前缀加说明，实际 %q", got)
	}
	if got := dlqExhaustedErrorMsg("boom"); got != dlqExhaustedErrorPrefix+"已达最大重试次数，boom" {
		t.Fatalf("普通错误期望追加前缀，实际 %q", got)
	}
	prefixed := dlqExhaustedErrorPrefix + "已达最大重试次数，boom"
	if got := dlqExhaustedErrorMsg(prefixed); got != prefixed {
		t.Fatalf("已带前缀的错误不应重复追加，实际 %q", got)
	}
}

// TestDlqHandlerStartRejectsNilFactory 验证缺少分布式锁工厂时启动报错
func TestDlqHandlerStartRejectsNilFactory(t *testing.T) {
	h, _, _, _, cleanup := newTestDlqHandler(t, &stubPublisher{}, passthroughExecutor{})
	defer cleanup()
	if err := h.Start(context.Background(), nil); err == nil {
		t.Fatal("nil 分布式锁工厂应返回错误")
	}
}
