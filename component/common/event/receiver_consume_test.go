// Package event 的幂等消费链路补充测试
//
// @author zwei
package event

import (
	"context"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/common/db/dbtest"
	"gitee.com/hq_project/yomi-boot/component/common/distlock"
	"gitee.com/hq_project/yomi-boot/component/common/event/cfg"
	"gitee.com/hq_project/yomi-boot/component/common/event/model"
	"gitee.com/hq_project/yomi-boot/component/common/event/repo"
	"gitee.com/hq_project/yomi-boot/component/common/redis/redistest"
	"gitee.com/hq_project/yomi-boot/component/common/snowflake/snowflaketest"

	yomikafka "gitee.com/hq_project/yomi-boot/component/common/kafka"
)

// TestReceiverWrapHandlerGuardBranches 覆盖幂等消费的前置校验与并发分支
func TestReceiverWrapHandlerGuardBranches(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(model.SysEventInbox{}, model.SysEventInboxDlq{}))
	defer cleanup()

	inboxRepo := repo.NewInboxRepo(gormDB)
	receiver := NewReceiver(
		inboxRepo,
		repo.NewInboxDlqRepo(gormDB),
		passthroughExecutor{},
		newTestLockFactory(t),
		cfg.Runtime{ServiceName: "svc", InboxDlqMaxRetry: 3},
		snowflaketest.NewTestGenerator(),
	)
	ctx := context.Background()
	validMsg := yomikafka.Message{
		Topic: "topic", Value: []byte(`{"eventId":"event-1","topic":"topic","payload":"payload-1"}`),
	}

	t.Run("action 未配置返回 Retry", func(t *testing.T) {
		if result := receiver.WrapHandler(nil)(ctx, validMsg); result != yomikafka.Retry() {
			t.Fatalf("未配置 action 应 Retry，实际 %+v", result)
		}
	})

	t.Run("空消息抛弃", func(t *testing.T) {
		handler := receiver.WrapHandler(func(context.Context, Event) error { return nil })
		if result := handler(ctx, yomikafka.Message{Value: nil}); result != yomikafka.AckDrop() {
			t.Fatalf("空消息应 AckDrop，实际 %+v", result)
		}
	})

	t.Run("非法消息抛弃", func(t *testing.T) {
		handler := receiver.WrapHandler(func(context.Context, Event) error { return nil })
		if result := handler(ctx, yomikafka.Message{Value: []byte("not-json")}); result != yomikafka.AckDrop() {
			t.Fatalf("非法消息应 AckDrop，实际 %+v", result)
		}
	})

	t.Run("重复消费幂等去重", func(t *testing.T) {
		handler := receiver.WrapHandler(func(context.Context, Event) error { return nil })
		if result := handler(ctx, validMsg); result != yomikafka.AckOK() {
			t.Fatalf("首次消费应 AckOK，实际 %+v", result)
		}
		if result := handler(ctx, validMsg); result != yomikafka.AckOK() {
			t.Fatalf("重复消费应 AckOK，实际 %+v", result)
		}
		var count int64
		if err := gormDB.Model(&model.SysEventInbox{}).Where("event_id = ?", "event-1").Count(&count).Error; err != nil {
			t.Fatalf("查询收件箱失败: %v", err)
		}
		if count != 1 {
			t.Fatalf("重复消费不应重复入库，实际 %d 条", count)
		}
	})

	t.Run("消费锁被他人持有", func(t *testing.T) {
		factory := newTestLockFactory(t)
		receiver := NewReceiver(
			inboxRepo,
			repo.NewInboxDlqRepo(gormDB),
			passthroughExecutor{},
			factory,
			cfg.Runtime{ServiceName: "svc", InboxDlqMaxRetry: 3},
			snowflaketest.NewTestGenerator(),
		)
		handler := receiver.WrapHandler(func(context.Context, Event) error { return nil })

		// 他方持锁且收件箱无记录：视为处理中，不 ack
		lock := factory.GetLock(receiver.cfg.LockKey("event-2"))
		held, err := lock.TryLock(ctx)
		if err != nil || !held {
			t.Fatalf("预占消费锁失败: held=%v err=%v", held, err)
		}
		msg := yomikafka.Message{
			Topic: "topic", Value: []byte(`{"eventId":"event-2","topic":"topic","payload":"p"}`),
		}
		if result := handler(ctx, msg); result != yomikafka.Retry() {
			t.Fatalf("他人持锁且未消费应 Retry，实际 %+v", result)
		}

		// 他方持锁但收件箱已有记录：视为重复，ack 跳过
		if err := inboxRepo.Insert(ctx, &model.SysEventInbox{
			ServiceName: "svc", EventID: "event-2", Topic: "topic", Payload: "p",
		}); err != nil {
			t.Fatalf("预置收件箱记录失败: %v", err)
		}
		if result := handler(ctx, msg); result != yomikafka.AckOK() {
			t.Fatalf("他人持锁但已消费应 AckOK，实际 %+v", result)
		}
		if err := lock.Unlock(ctx); err != nil {
			t.Fatalf("释放预占锁失败: %v", err)
		}
	})

	t.Run("消费锁获取失败返回 Retry", func(t *testing.T) {
		client, redisCleanup := redistest.NewClient(t)
		factory, err := distlock.NewFactory(client)
		if err != nil {
			t.Fatalf("创建分布式锁工厂失败: %v", err)
		}
		redisCleanup()
		receiver := NewReceiver(
			inboxRepo,
			repo.NewInboxDlqRepo(gormDB),
			passthroughExecutor{},
			factory,
			cfg.Runtime{ServiceName: "svc", InboxDlqMaxRetry: 3},
			snowflaketest.NewTestGenerator(),
		)
		handler := receiver.WrapHandler(func(context.Context, Event) error { return nil })
		if result := handler(ctx, validMsg); result != yomikafka.Retry() {
			t.Fatalf("Redis 不可用应 Retry，实际 %+v", result)
		}
	})
}
