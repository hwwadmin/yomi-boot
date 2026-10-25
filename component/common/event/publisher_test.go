// Package event 的发件箱事件发布器测试
//
// @author zwei
package event

import (
	"context"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/common/db/dbtest"
	"gitee.com/hq_project/yomi-boot/component/common/event/cfg"
	"gitee.com/hq_project/yomi-boot/component/common/event/model"
	"gitee.com/hq_project/yomi-boot/component/common/event/repo"
	"gitee.com/hq_project/yomi-boot/component/common/snowflake/snowflaketest"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/shared/enums"
)

// TestPublisherPublish 验证事件写入发件箱与参数校验
func TestPublisherPublish(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(model.SysEventOutbox{}))
	defer cleanup()

	p := NewPublisher(
		repo.NewOutboxRepo(gormDB),
		cfg.Runtime{ServiceName: "svc"},
		snowflaketest.NewTestGenerator(),
		enums.MySQL,
	)
	ctx := context.Background()

	if err := p.Publish(ctx, Event{EventID: "event-1", Topic: "topic", Payload: "payload-1"}); err != nil {
		t.Fatalf("发布事件失败: %v", err)
	}
	var stored model.SysEventOutbox
	if err := gormDB.First(&stored, "event_id = ?", "event-1").Error; err != nil {
		t.Fatalf("查询发件箱失败: %v", err)
	}
	if stored.ServiceName != "svc" || stored.Topic != "topic" || stored.Payload != "payload-1" {
		t.Fatalf("发件箱字段不符: %+v", stored)
	}
	if stored.Status != model.OutboxStatusPending || stored.ID == 0 {
		t.Fatalf("新事件应为 PENDING 且带雪花 ID: %+v", stored)
	}

	if err := p.Publish(ctx, Event{Topic: "topic"}); !errors.Is(err, ErrEventInvalid) {
		t.Fatalf("空 eventId 应返回 ErrEventInvalid，实际 %v", err)
	}
	if err := p.Publish(ctx, Event{EventID: "event-2"}); !errors.Is(err, ErrEventInvalid) {
		t.Fatalf("空 topic 应返回 ErrEventInvalid，实际 %v", err)
	}
}

// TestPublisherPublishDuplicate 验证同一服务下重复 eventId 返回 ErrDuplicatePublish
func TestPublisherPublishDuplicate(t *testing.T) {
	gormDB, cleanup := dbtest.New(t,
		dbtest.WithTables(model.SysEventOutbox{}),
		dbtest.WithSQL("ALTER TABLE sys_event_outbox ADD UNIQUE KEY uk_outbox_svc_event (service_name, event_id)"),
	)
	defer cleanup()

	p := NewPublisher(
		repo.NewOutboxRepo(gormDB),
		cfg.Runtime{ServiceName: "svc"},
		snowflaketest.NewTestGenerator(),
		enums.MySQL,
	)
	ctx := context.Background()
	evt := Event{EventID: "event-1", Topic: "topic"}
	if err := p.Publish(ctx, evt); err != nil {
		t.Fatalf("首次发布失败: %v", err)
	}
	if err := p.Publish(ctx, evt); !errors.Is(err, ErrDuplicatePublish) {
		t.Fatalf("重复发布应返回 ErrDuplicatePublish，实际 %v", err)
	}
}

// TestPublisherPublishIDGenFailure 验证 ID 生成失败时错误透传
func TestPublisherPublishIDGenFailure(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(model.SysEventOutbox{}))
	defer cleanup()

	p := NewPublisher(
		repo.NewOutboxRepo(gormDB),
		cfg.Runtime{ServiceName: "svc"},
		errIDGen{err: errTestPublish},
		enums.MySQL,
	)
	if err := p.Publish(context.Background(), Event{EventID: "event-1", Topic: "topic"}); err == nil {
		t.Fatal("ID 生成失败应返回错误")
	}
}
