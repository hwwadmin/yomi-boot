// Package outbox 的发件箱抓取器测试
//
// @author zwei
package outbox

import (
	"context"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/event/cfg"
	"gitee.com/hq_project/yomi-boot/component/common/event/model"
)

// TestFetcherDoTask 验证抓取 PENDING 事件、标记 FETCHED 并提交发送
func TestFetcherDoTask(t *testing.T) {
	pub := &stubPublisher{}
	w := &syncWorker{}
	p, gormDB, cleanup := newTestProcessor(t, w, pub)
	defer cleanup()

	rows := []model.SysEventOutbox{
		{ServiceName: "svc", EventID: "event-1", Topic: "topic", Status: model.OutboxStatusPending},
		{ServiceName: "svc", EventID: "event-2", Topic: "topic", Status: model.OutboxStatusPending},
		{ServiceName: "other", EventID: "event-3", Topic: "topic", Status: model.OutboxStatusPending},
		{ServiceName: "svc", EventID: "event-4", Topic: "topic", Status: model.OutboxStatusSentSuccess},
	}
	for i := range rows {
		rows[i].ID = int64(i + 1)
	}
	if err := gormDB.Create(&rows).Error; err != nil {
		t.Fatalf("初始化发件箱数据失败: %v", err)
	}

	f := NewFetcher(cfg.Runtime{ServiceName: "svc", FetchSize: 10}, p.outboxRepo, p)
	if err := f.doTask(context.Background()); err != nil {
		t.Fatalf("抓取任务失败: %v", err)
	}

	if got := w.submitted(); got != 2 {
		t.Fatalf("期望提交 2 条本服务 PENDING 事件，实际 %d 条", got)
	}
	if pub.calls() != 2 {
		t.Fatalf("期望推送 2 次，实际 %d 次", pub.calls())
	}
	var stored []model.SysEventOutbox
	if err := gormDB.Order("id ASC").Find(&stored).Error; err != nil {
		t.Fatalf("查询发件箱失败: %v", err)
	}
	if stored[0].Status != model.OutboxStatusSentSuccess || stored[0].SentTime == nil {
		t.Fatalf("event-1 应发送成功: %+v", stored[0])
	}
	if stored[1].Status != model.OutboxStatusSentSuccess {
		t.Fatalf("event-2 应发送成功: %+v", stored[1])
	}
	if stored[2].Status != model.OutboxStatusPending {
		t.Fatalf("其他服务事件不应被抓取: %+v", stored[2])
	}
	if stored[3].Status != model.OutboxStatusSentSuccess {
		t.Fatalf("已成功事件不应被抓取: %+v", stored[3])
	}
}

// TestFetcherDoTaskSkipsWhenNoCapacity 验证无抓取配额时直接返回
func TestFetcherDoTaskSkipsWhenNoCapacity(t *testing.T) {
	t.Run("配置抓取量为 0", func(t *testing.T) {
		p, gormDB, cleanup := newTestProcessor(t, &syncWorker{}, &stubPublisher{})
		defer cleanup()
		mustInsertFetched(t, gormDB, 1, "event-1")

		f := &Fetcher{cfg: cfg.Runtime{ServiceName: "svc", FetchSize: 0}, outboxRepo: p.outboxRepo, processor: p}
		if err := f.doTask(context.Background()); err != nil {
			t.Fatalf("抓取任务不应报错: %v", err)
		}
		var count int64
		if err := gormDB.Model(&model.SysEventOutbox{}).Count(&count).Error; err != nil {
			t.Fatalf("查询发件箱失败: %v", err)
		}
		if count != 1 {
			t.Fatalf("不应产生额外数据，实际 %d 条", count)
		}
	})

	t.Run("发送队列已满", func(t *testing.T) {
		pub := &stubPublisher{}
		p, gormDB, cleanup := newTestProcessor(t, &fullWorker{pending: 256}, pub)
		defer cleanup()
		mustInsertFetched(t, gormDB, 1, "event-1")

		f := &Fetcher{cfg: cfg.Runtime{ServiceName: "svc", FetchSize: 10}, outboxRepo: p.outboxRepo, processor: p}
		if err := f.doTask(context.Background()); err != nil {
			t.Fatalf("抓取任务不应报错: %v", err)
		}
		if pub.calls() != 0 {
			t.Fatalf("队列已满不应推送，实际推送 %d 次", pub.calls())
		}
		var stored model.SysEventOutbox
		if err := gormDB.First(&stored, 1).Error; err != nil {
			t.Fatalf("查询发件箱失败: %v", err)
		}
		if stored.Status != model.OutboxStatusFetched {
			t.Fatalf("队列已满不应改变记录状态，实际 %s", stored.Status)
		}
	})
}

// TestFetcherStartRejectsNilFactory 验证缺少分布式锁工厂时启动报错
func TestFetcherStartRejectsNilFactory(t *testing.T) {
	p, _, cleanup := newTestProcessor(t, &syncWorker{}, &stubPublisher{})
	defer cleanup()
	f := NewFetcher(cfg.Runtime{ServiceName: "svc", FetchFrequency: 50 * time.Millisecond}, p.outboxRepo, p)
	if err := f.Start(context.Background(), nil); err == nil {
		t.Fatal("nil 分布式锁工厂应返回错误")
	}
}
