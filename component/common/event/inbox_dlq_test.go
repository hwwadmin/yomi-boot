// Package event 的收件箱死信后台重投测试
//
// @author zwei
package event

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hwwadmin/yomi-boot/component/common/db/dbtest"
	"github.com/hwwadmin/yomi-boot/component/common/event/cfg"
	"github.com/hwwadmin/yomi-boot/component/common/event/model"
	"github.com/hwwadmin/yomi-boot/component/common/event/repo"

	yomikafka "github.com/hwwadmin/yomi-boot/component/common/kafka"
)

// errTestPublish 是测试用发布失败错误
var errTestPublish = errors.New("publish failed")

// stubPublisher 仅记录 PublishJSON 调用的 Kafka 发布者桩
type stubPublisher struct {
	err       error
	calls     int
	topic     string
	key       string
	published model.WireEvent
}

// Publish 空实现
func (s *stubPublisher) Publish(context.Context, yomikafka.PublishRequest) error { return nil }

// PublishBatch 空实现
func (s *stubPublisher) PublishBatch(context.Context, []yomikafka.PublishRequest) error {
	return nil
}

// PublishJSON 记录发布内容并按预设错误返回
func (s *stubPublisher) PublishJSON(_ context.Context, topic string, key string, value any) error {
	s.calls++
	s.topic = topic
	s.key = key
	if evt, ok := value.(model.WireEvent); ok {
		s.published = evt
	}
	return s.err
}

// PublishAsync 空实现
func (s *stubPublisher) PublishAsync(context.Context, yomikafka.PublishRequest) error { return nil }

// PublishBatchAsync 空实现
func (s *stubPublisher) PublishBatchAsync(context.Context, []yomikafka.PublishRequest) error {
	return nil
}

// PublishJSONAsync 空实现
func (s *stubPublisher) PublishJSONAsync(context.Context, string, string, any) error { return nil }

// Close 空实现
func (s *stubPublisher) Close() error { return nil }

// TestInboxDlqHandlerReplaySuccess 验证死信重新投递成功后删除记录
func TestInboxDlqHandlerReplaySuccess(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(model.SysEventInbox{}, model.SysEventInboxDlq{}))
	defer cleanup()

	dlqRepo := repo.NewInboxDlqRepo(gormDB)
	inboxRepo := repo.NewInboxRepo(gormDB)
	row := &model.SysEventInboxDlq{
		ServiceName: "svc",
		EventID:     "event-1",
		Topic:       "topic",
		Payload:     "payload-1",
		RetryCount:  20,
	}
	row.ID = 1
	if err := gormDB.Create(row).Error; err != nil {
		t.Fatalf("初始化死信失败: %v", err)
	}

	publisher := &stubPublisher{}
	handler := NewInboxDlqHandler(
		cfg.Runtime{ServiceName: "svc", InboxDlqFrequency: 30 * time.Second, InboxDlqSize: 100},
		dlqRepo, inboxRepo, publisher,
	)
	handler.processOne(context.Background(), row)

	if publisher.calls != 1 || publisher.topic != "topic" || publisher.key != "event-1" {
		t.Fatalf("发布调用不符: %+v", publisher)
	}
	if publisher.published.Payload != "payload-1" {
		t.Fatalf("发布 payload 不符: %+v", publisher.published)
	}
	var count int64
	if err := gormDB.Model(&model.SysEventInboxDlq{}).Count(&count).Error; err != nil {
		t.Fatalf("查询死信失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("重投成功后死信应删除，实际 %d 条", count)
	}
}

// TestInboxDlqHandlerReplayFailure 验证重新投递失败后累加重试次数并推迟下次重投
func TestInboxDlqHandlerReplayFailure(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(model.SysEventInbox{}, model.SysEventInboxDlq{}))
	defer cleanup()

	dlqRepo := repo.NewInboxDlqRepo(gormDB)
	inboxRepo := repo.NewInboxRepo(gormDB)
	row := &model.SysEventInboxDlq{
		ServiceName: "svc",
		EventID:     "event-1",
		Topic:       "topic",
		Payload:     "payload-1",
		RetryCount:  20,
	}
	row.ID = 1
	if err := gormDB.Create(row).Error; err != nil {
		t.Fatalf("初始化死信失败: %v", err)
	}

	publisher := &stubPublisher{err: errTestPublish}
	handler := NewInboxDlqHandler(
		cfg.Runtime{ServiceName: "svc", InboxDlqFrequency: 30 * time.Second, InboxDlqSize: 100},
		dlqRepo, inboxRepo, publisher,
	)
	before := time.Now()
	handler.processOne(context.Background(), row)

	var stored model.SysEventInboxDlq
	if err := gormDB.First(&stored, "id = ?", row.ID).Error; err != nil {
		t.Fatalf("查询死信失败: %v", err)
	}
	if stored.RetryCount != 21 {
		t.Fatalf("重试次数期望 21，实际 %d", stored.RetryCount)
	}
	if stored.RetryLastTime == nil || stored.RetryAfterTime == nil {
		t.Fatalf("重投时间字段不应为空: %+v", stored)
	}
	if stored.RetryAfterTime.Before(before.Add(29 * time.Second)) {
		t.Fatalf("下次重投时间过早: %v", stored.RetryAfterTime)
	}
}

// TestInboxDlqHandlerSkipsConsumedEvent 验证已消费事件直接清理死信，不再重投
func TestInboxDlqHandlerSkipsConsumedEvent(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(model.SysEventInbox{}, model.SysEventInboxDlq{}))
	defer cleanup()

	dlqRepo := repo.NewInboxDlqRepo(gormDB)
	inboxRepo := repo.NewInboxRepo(gormDB)
	row := &model.SysEventInboxDlq{
		ServiceName: "svc",
		EventID:     "event-1",
		Topic:       "topic",
		Payload:     "payload-1",
		RetryCount:  20,
	}
	row.ID = 1
	inbox := &model.SysEventInbox{ServiceName: "svc", EventID: "event-1", Topic: "topic", Payload: "payload-1"}
	inbox.ID = 2
	if err := gormDB.Create(row).Error; err != nil {
		t.Fatalf("初始化死信失败: %v", err)
	}
	if err := gormDB.Create(inbox).Error; err != nil {
		t.Fatalf("初始化收件箱失败: %v", err)
	}

	publisher := &stubPublisher{}
	handler := NewInboxDlqHandler(
		cfg.Runtime{ServiceName: "svc", InboxDlqFrequency: 30 * time.Second, InboxDlqSize: 100},
		dlqRepo, inboxRepo, publisher,
	)
	handler.processOne(context.Background(), row)

	if publisher.calls != 0 {
		t.Fatalf("已消费事件不应重投，实际调用 %d 次", publisher.calls)
	}
	var count int64
	if err := gormDB.Model(&model.SysEventInboxDlq{}).Count(&count).Error; err != nil {
		t.Fatalf("查询死信失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("已消费事件的死信应删除，实际 %d 条", count)
	}
}

// TestInboxDlqHandlerProcessDlq 验证从拉取到期死信到逐条重投的整体流程
func TestInboxDlqHandlerProcessDlq(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(model.SysEventInbox{}, model.SysEventInboxDlq{}))
	defer cleanup()

	dlqRepo := repo.NewInboxDlqRepo(gormDB)
	inboxRepo := repo.NewInboxRepo(gormDB)
	unconsumed := &model.SysEventInboxDlq{ServiceName: "svc", EventID: "event-1", Topic: "topic", Payload: "payload-1"}
	unconsumed.ID = 1
	consumed := &model.SysEventInboxDlq{ServiceName: "svc", EventID: "event-2", Topic: "topic", Payload: "payload-2"}
	consumed.ID = 2
	inbox := &model.SysEventInbox{ServiceName: "svc", EventID: "event-2", Topic: "topic", Payload: "payload-2"}
	inbox.ID = 3
	for _, row := range []*model.SysEventInboxDlq{unconsumed, consumed} {
		if err := gormDB.Create(row).Error; err != nil {
			t.Fatalf("初始化死信失败: %v", err)
		}
	}
	if err := gormDB.Create(inbox).Error; err != nil {
		t.Fatalf("初始化收件箱失败: %v", err)
	}

	publisher := &stubPublisher{}
	handler := NewInboxDlqHandler(
		cfg.Runtime{ServiceName: "svc", InboxDlqFrequency: 30 * time.Second, InboxDlqSize: 100},
		dlqRepo, inboxRepo, publisher,
	)
	if err := handler.processDlq(context.Background()); err != nil {
		t.Fatalf("处理死信失败: %v", err)
	}

	if publisher.calls != 1 || publisher.key != "event-1" {
		t.Fatalf("仅未消费事件应重投，实际调用 %+v", publisher)
	}
	var count int64
	if err := gormDB.Model(&model.SysEventInboxDlq{}).Count(&count).Error; err != nil {
		t.Fatalf("查询死信失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("重投与清理完成后死信应清空，实际 %d 条", count)
	}
}

// TestInboxDlqHandlerStartValidation 验证启动时的依赖校验与降级分支
func TestInboxDlqHandlerStartValidation(t *testing.T) {
	t.Run("仓储缺失返回错误", func(t *testing.T) {
		handler := NewInboxDlqHandler(cfg.Runtime{ServiceName: "svc"}, nil, nil, &stubPublisher{})
		if err := handler.Start(context.Background(), &stubDistLockFactory{}); err == nil {
			t.Fatal("收件箱死信仓储缺失应返回错误")
		}
	})

	t.Run("发布者缺失降级不启动", func(t *testing.T) {
		gormDB, cleanup := dbtest.New(t, dbtest.WithTables(model.SysEventInbox{}, model.SysEventInboxDlq{}))
		defer cleanup()
		handler := NewInboxDlqHandler(
			cfg.Runtime{ServiceName: "svc", InboxDlqFrequency: time.Second},
			repo.NewInboxDlqRepo(gormDB), repo.NewInboxRepo(gormDB), nil,
		)
		if err := handler.Start(context.Background(), &stubDistLockFactory{}); err != nil {
			t.Fatalf("发布者缺失应降级为不启动，实际 %v", err)
		}
	})
}

// TestInboxDlqHandler_DeadRepoErrors 验证启动失败、查询失败以及重投前后的仓储错误
func TestInboxDlqHandler_DeadRepoErrors(t *testing.T) {
	db := newDeadDB(t)
	dlqRepo := repo.NewInboxDlqRepo(db)
	inboxRepo := repo.NewInboxRepo(db)
	ctx := context.Background()

	handler := NewInboxDlqHandler(cfg.Runtime{
		ServiceName:       "svc",
		InboxDlqFrequency: time.Millisecond,
		InboxDlqSize:      1,
	}, dlqRepo, inboxRepo, &stubPublisher{})
	if err := handler.Start(ctx, &stubDistLockFactory{}); err == nil {
		t.Fatal("轮询频率过低应启动失败")
	}
	if err := handler.processDlq(ctx); err == nil {
		t.Fatal("查询收件箱死信失败应返回错误")
	}
	handler.processOne(ctx, &model.SysEventInboxDlq{ServiceName: "svc", EventID: "event-1", Topic: "topic"})

	failHandler := NewInboxDlqHandler(cfg.Runtime{ServiceName: "svc", InboxDlqFrequency: time.Second}, dlqRepo, inboxRepo, &stubPublisher{err: errTestPublish})
	failHandler.processOne(ctx, &model.SysEventInboxDlq{Topic: "topic"})

	okHandler := NewInboxDlqHandler(cfg.Runtime{ServiceName: "svc"}, dlqRepo, inboxRepo, &stubPublisher{})
	okHandler.processOne(ctx, &model.SysEventInboxDlq{EventID: "event-2", Topic: "topic"})
}
