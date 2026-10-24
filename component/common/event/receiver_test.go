// Package event 的收件箱死信与消费重试测试
//
// @author zwei
package event

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"gitee.com/hq_project/yomi-boot/component/common/db/dbtest"
	"gitee.com/hq_project/yomi-boot/component/common/db/tx"
	"gitee.com/hq_project/yomi-boot/component/common/distlock"
	"gitee.com/hq_project/yomi-boot/component/common/event/cfg"
	"gitee.com/hq_project/yomi-boot/component/common/event/model"
	"gitee.com/hq_project/yomi-boot/component/common/event/repo"
	"gitee.com/hq_project/yomi-boot/component/common/redis/redistest"
	"gitee.com/hq_project/yomi-boot/component/common/snowflake"

	yomikafka "gitee.com/hq_project/yomi-boot/component/common/kafka"
)

// TestReceiverShouldMoveToInboxDlq 验证死信转存阈值与无限重试配置
func TestReceiverShouldMoveToInboxDlq(t *testing.T) {
	dlqRepo := &repo.InboxDlqRepo{}
	cases := []struct {
		name       string
		repo       *repo.InboxDlqRepo
		maxRetry   int
		retryCount int
		want       bool
	}{
		{name: "below threshold", repo: dlqRepo, maxRetry: 2, retryCount: 0, want: false},
		{name: "last retry", repo: dlqRepo, maxRetry: 2, retryCount: 1, want: false},
		{name: "at threshold", repo: dlqRepo, maxRetry: 2, retryCount: 2, want: true},
		{name: "above threshold", repo: dlqRepo, maxRetry: 2, retryCount: 3, want: true},
		{name: "zero retry", repo: dlqRepo, maxRetry: 0, retryCount: 0, want: true},
		{name: "infinite retry", repo: dlqRepo, maxRetry: -1, retryCount: 100, want: false},
		{name: "no dlq repo", repo: nil, maxRetry: 2, retryCount: 100, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &Receiver{
				inboxDlqRepo: tc.repo,
				cfg:          cfg.Runtime{InboxDlqMaxRetry: tc.maxRetry},
			}
			if got := r.shouldMoveToInboxDlq(tc.retryCount); got != tc.want {
				t.Fatalf("shouldMoveToInboxDlq 期望 %v，实际 %v", tc.want, got)
			}
		})
	}
}

// TestMessageRetryTracker 验证同一消息的失败计数与重置
func TestMessageRetryTracker(t *testing.T) {
	tracker := newMessageRetryTracker()
	key := messageRetryKey{topic: "topic", partition: 1, offset: 10}

	for i := 1; i <= 3; i++ {
		if got := tracker.increment(key); got != i {
			t.Fatalf("第 %d 次 increment 期望 %d，实际 %d", i, i, got)
		}
	}
	tracker.reset(key)
	if got := tracker.increment(key); got != 1 {
		t.Fatalf("reset 后 increment 期望 1，实际 %d", got)
	}
}

// TestTruncateInboxDlqError 验证死信错误信息按字符截断且不破坏 UTF-8
func TestTruncateInboxDlqError(t *testing.T) {
	if got := truncateInboxDlqError(nil); got != "" {
		t.Fatalf("nil error 期望空串，实际 %q", got)
	}
	short := fmt.Errorf("boom")
	if got := truncateInboxDlqError(short); got != "boom" {
		t.Fatalf("短错误期望原样返回，实际 %q", got)
	}

	long := fmt.Errorf("%s", strings.Repeat("错", inboxDlqErrorMsgLimit+10))
	got := truncateInboxDlqError(long)
	if !utf8.ValidString(got) {
		t.Fatal("截断后的错误信息不是合法 UTF-8")
	}
	if !strings.HasSuffix(got, "...[truncated]") {
		t.Fatalf("截断后的错误信息缺少截断标记: %q", got)
	}
	if utf8.RuneCountInString(got) != inboxDlqErrorMsgLimit+len("...[truncated]") {
		t.Fatalf("截断长度不符，实际 %d", utf8.RuneCountInString(got))
	}
}

// TestConfigValidateInboxDlqMaxRetry 验证消费最大重试配置的合法域
func TestConfigValidateInboxDlqMaxRetry(t *testing.T) {
	cases := []struct {
		name    string
		value   int
		wantErr bool
	}{
		{name: "infinite", value: -1},
		{name: "no retry", value: 0},
		{name: "finite", value: 16},
		{name: "invalid", value: -2, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value := tc.value
			config := Config{InboxEnabled: true, InboxDlqMaxRetry: &value}
			err := config.Validate()
			if tc.wantErr && err == nil {
				t.Fatal("期望配置校验失败")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("期望配置校验通过，实际 %v", err)
			}
		})
	}
}

// TestInvokeEventHandlerRecoversPanic 验证业务 panic 被转为普通错误，便于进入重试计数
func TestInvokeEventHandlerRecoversPanic(t *testing.T) {
	err := invokeEventHandler(func(context.Context, Event) error {
		panic("boom")
	}, context.Background(), Event{})
	if err == nil {
		t.Fatal("业务 panic 应转为错误")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("错误信息应包含 panic 内容，实际 %q", err.Error())
	}
}

// TestReceiverMovesToInboxDlqAfterRetryExhausted 验证业务连续失败后写入死信并 ack
func TestReceiverMovesToInboxDlqAfterRetryExhausted(t *testing.T) {
	gormDB, cleanup := dbtest.New(t,
		dbtest.WithTables(model.SysEventInbox{}, model.SysEventInboxDlq{}),
	)
	defer cleanup()

	client, redisCleanup := redistest.NewClient(t)
	defer redisCleanup()

	factory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("创建分布式锁工厂失败: %v", err)
	}
	gen, err := snowflake.NewGenerator(1)
	if err != nil {
		t.Fatalf("创建雪花 ID 生成器失败: %v", err)
	}

	receiver := NewReceiver(
		repo.NewInboxRepo(gormDB),
		repo.NewInboxDlqRepo(gormDB),
		tx.NewManager(gormDB),
		factory,
		cfg.Runtime{ServiceName: "svc", InboxDlqMaxRetry: 2},
		gen,
	)
	handler := receiver.WrapHandler(func(context.Context, Event) error {
		return fmt.Errorf("business failed")
	})
	msg := yomikafka.Message{
		Topic:     "topic",
		Partition: 0,
		Offset:    1,
		Value:     []byte(`{"eventId":"event-1","topic":"topic","payload":"payload-1"}`),
	}
	ctx := context.Background()

	for attempt := 1; attempt <= 2; attempt++ {
		if result := handler(ctx, msg); result.Ack {
			t.Fatalf("第 %d 次失败不应 ack", attempt)
		}
	}
	if result := handler(ctx, msg); !result.Ack {
		t.Fatal("达到最大重试次数后应 ack 并转入死信")
	}

	var dlqRows []model.SysEventInboxDlq
	if err := gormDB.Find(&dlqRows).Error; err != nil {
		t.Fatalf("查询收件箱死信失败: %v", err)
	}
	if len(dlqRows) != 1 {
		t.Fatalf("期望 1 条死信，实际 %d 条", len(dlqRows))
	}
	if dlqRows[0].EventID != "event-1" || dlqRows[0].RetryCount != 2 {
		t.Fatalf("死信内容不符: %+v", dlqRows[0])
	}
	if !strings.Contains(dlqRows[0].ErrorMsg, "business failed") {
		t.Fatalf("死信错误信息不符: %q", dlqRows[0].ErrorMsg)
	}

	var inboxCount int64
	if err := gormDB.Model(&model.SysEventInbox{}).Count(&inboxCount).Error; err != nil {
		t.Fatalf("查询收件箱失败: %v", err)
	}
	if inboxCount != 0 {
		t.Fatalf("业务事务应回滚，inbox 期望 0 条，实际 %d 条", inboxCount)
	}
}
