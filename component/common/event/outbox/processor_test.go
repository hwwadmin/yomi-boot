// Package outbox 的发件箱抓取与发送容量测试
//
// @author zwei
package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/event/cfg"
	"gitee.com/hq_project/yomi-boot/component/common/event/model"
	"gitee.com/hq_project/yomi-boot/component/common/event/repo"
	"gitee.com/hq_project/yomi-boot/component/common/snowflake/snowflaketest"
	"gitee.com/hq_project/yomi-boot/component/common/worker"

	"gorm.io/gorm"
)

// stubWorker 仅实现容量查询所需方法的工作池桩
type stubWorker struct {
	pending int
}

// Start 空实现
func (s *stubWorker) Start() {}

// Submit 空实现
func (s *stubWorker) Submit(worker.WorkerHandler) error { return nil }

// SubmitContext 空实现
func (s *stubWorker) SubmitContext(context.Context, worker.WorkerHandler) error { return nil }

// TrySubmit 空实现
func (s *stubWorker) TrySubmit(worker.WorkerHandler) error { return nil }

// Pending 返回预设的待处理任务数
func (s *stubWorker) Pending() int { return s.pending }

// Close 空实现
func (s *stubWorker) Close() error { return nil }

// Wait 空实现
func (s *stubWorker) Wait() {}

// TestProcessorAvailableCapacity 验证发送队列可用容量的计算与下限保护
func TestProcessorAvailableCapacity(t *testing.T) {
	cases := []struct {
		name    string
		queue   int
		pending int
		want    int
	}{
		{name: "empty", queue: 256, pending: 0, want: 256},
		{name: "partially used", queue: 256, pending: 156, want: 100},
		{name: "full", queue: 256, pending: 256, want: 0},
		{name: "over capacity", queue: 256, pending: 300, want: 0},
		{name: "zero queue", queue: 0, pending: 0, want: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &Processor{queueSize: tc.queue, worker: &stubWorker{pending: tc.pending}}
			if got := p.Capacity(); got != tc.queue {
				t.Fatalf("Capacity 期望 %d，实际 %d", tc.queue, got)
			}
			if got := p.Pending(); got != tc.pending {
				t.Fatalf("Pending 期望 %d，实际 %d", tc.pending, got)
			}
			if got := p.AvailableCapacity(); got != tc.want {
				t.Fatalf("AvailableCapacity 期望 %d，实际 %d", tc.want, got)
			}
		})
	}
}

// TestFetcherFetchSize 验证抓取量不超过发送队列可用容量
func TestFetcherFetchSize(t *testing.T) {
	cases := []struct {
		name       string
		configured int
		queue      int
		pending    int
		want       int
	}{
		{name: "configured smaller", configured: 50, queue: 256, pending: 0, want: 50},
		{name: "capacity smaller", configured: 5000, queue: 256, pending: 0, want: 256},
		{name: "available smaller", configured: 5000, queue: 256, pending: 200, want: 56},
		{name: "queue full", configured: 5000, queue: 256, pending: 256, want: 0},
		{name: "zero configured", configured: 0, queue: 256, pending: 0, want: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &Processor{queueSize: tc.queue, worker: &stubWorker{pending: tc.pending}}
			f := &Fetcher{cfg: cfg.Runtime{FetchSize: tc.configured}, processor: p}
			if got := f.fetchSize(); got != tc.want {
				t.Fatalf("fetchSize 期望 %d，实际 %d", tc.want, got)
			}
		})
	}
}

// newTestProcessor 构造带真实仓储与指定工作池的发送处理器
func newTestProcessor(
	t *testing.T,
	w worker.WorkerService,
	pub *stubPublisher,
) (*Processor, *gorm.DB, func()) {
	t.Helper()
	gormDB, outboxRepo, dlqRepo, cleanup := newOutboxTestDB(t)
	p := &Processor{
		cfg:            cfg.Runtime{ServiceName: "svc"},
		outboxRepo:     outboxRepo,
		dlq:            NewDlqHandler(cfg.Runtime{ServiceName: "svc"}, outboxRepo, dlqRepo, pub, passthroughExecutor{}, snowflaketest.NewTestGenerator()),
		kafkaPublisher: pub,
		txExecutor:     passthroughExecutor{},
		worker:         w,
		queueSize:      256,
	}
	return p, gormDB, cleanup
}

// mustInsertFetched 写入一条 FETCHED 状态的发件箱记录并返回
func mustInsertFetched(t *testing.T, gormDB *gorm.DB, id int64, eventID string) model.SysEventOutbox {
	t.Helper()
	fetchTime := time.Now().Add(-time.Minute)
	row := model.SysEventOutbox{
		ServiceName: "svc", EventID: eventID, Topic: "topic",
		Status: model.OutboxStatusFetched, FetchTime: &fetchTime,
	}
	row.ID = id
	if err := gormDB.Create(&row).Error; err != nil {
		t.Fatalf("初始化发件箱数据失败: %v", err)
	}
	return row
}

// TestProcessorNilSafe 验证空处理器的容量查询不 panic
func TestProcessorNilSafe(t *testing.T) {
	var p *Processor
	if got := p.Capacity(); got != 0 {
		t.Fatalf("nil 处理器 Capacity 期望 0，实际 %d", got)
	}
	if got := p.Pending(); got != 0 {
		t.Fatalf("nil 处理器 Pending 期望 0，实际 %d", got)
	}
	if got := p.AvailableCapacity(); got != 0 {
		t.Fatalf("nil 处理器 AvailableCapacity 期望 0，实际 %d", got)
	}
}

// TestNewProcessorValidation 验证必填依赖校验
func TestNewProcessorValidation(t *testing.T) {
	ctx := context.Background()
	runtime := cfg.Runtime{ServiceName: "svc"}
	pub := &stubPublisher{}
	_, outboxRepo, dlqRepo, cleanup := newOutboxTestDB(t)
	defer cleanup()
	dlq := NewDlqHandler(runtime, outboxRepo, dlqRepo, pub, passthroughExecutor{}, snowflaketest.NewTestGenerator())

	if _, err := NewProcessor(runtime, nil, dlq, pub, passthroughExecutor{}, ctx); err == nil {
		t.Fatal("outboxRepo 为空应返回错误")
	}
	if _, err := NewProcessor(runtime, outboxRepo, nil, pub, passthroughExecutor{}, ctx); err == nil {
		t.Fatal("dlq 为空应返回错误")
	}
	if _, err := NewProcessor(runtime, outboxRepo, dlq, nil, passthroughExecutor{}, ctx); err == nil {
		t.Fatal("kafkaPublisher 为空应返回错误")
	}
	if _, err := NewProcessor(runtime, outboxRepo, dlq, pub, nil, ctx); err == nil {
		t.Fatal("txExecutor 为空应返回错误")
	}
}

// TestNewProcessorAndClose 验证正常创建与幂等关闭
func TestNewProcessorAndClose(t *testing.T) {
	_, outboxRepo, dlqRepo, cleanup := newOutboxTestDB(t)
	defer cleanup()

	p, err := NewProcessor(
		cfg.Runtime{ServiceName: "svc"},
		outboxRepo,
		NewDlqHandler(cfg.Runtime{ServiceName: "svc"}, outboxRepo, dlqRepo, &stubPublisher{}, passthroughExecutor{}, snowflaketest.NewTestGenerator()),
		&stubPublisher{},
		passthroughExecutor{},
		context.Background(),
	)
	if err != nil {
		t.Fatalf("创建处理器失败: %v", err)
	}
	if p.Capacity() != 256 {
		t.Fatalf("默认队列容量期望 256，实际 %d", p.Capacity())
	}
	p.Close()
	p.Close()
}

// TestProcessorPush 验证推送成功回写与失败写死信
func TestProcessorPush(t *testing.T) {
	ctx := context.Background()

	t.Run("推送成功", func(t *testing.T) {
		pub := &stubPublisher{}
		p, gormDB, cleanup := newTestProcessor(t, &syncWorker{}, pub)
		defer cleanup()
		row := mustInsertFetched(t, gormDB, 1, "event-1")

		if err := p.push(ctx, row); err != nil {
			t.Fatalf("推送不应报错: %v", err)
		}
		if pub.calls() != 1 {
			t.Fatalf("期望推送 1 次，实际 %d 次", pub.calls())
		}
		var stored model.SysEventOutbox
		if err := gormDB.First(&stored, row.ID).Error; err != nil {
			t.Fatalf("查询发件箱失败: %v", err)
		}
		if stored.Status != model.OutboxStatusSentSuccess || stored.SentTime == nil {
			t.Fatalf("发件箱应回写为发送成功: %+v", stored)
		}
	})

	t.Run("推送失败写死信", func(t *testing.T) {
		pub := &stubPublisher{err: errors.New("boom")}
		p, gormDB, cleanup := newTestProcessor(t, &syncWorker{}, pub)
		defer cleanup()
		row := mustInsertFetched(t, gormDB, 1, "event-1")

		if err := p.push(ctx, row); err == nil {
			t.Fatal("推送失败应返回错误")
		}
		var stored model.SysEventOutbox
		if err := gormDB.First(&stored, row.ID).Error; err != nil {
			t.Fatalf("查询发件箱失败: %v", err)
		}
		if stored.Status != model.OutboxStatusSentFail {
			t.Fatalf("发件箱应标记为发送失败，实际 %s", stored.Status)
		}
		var dlqRows []model.SysEventOutboxDlq
		if err := gormDB.Find(&dlqRows).Error; err != nil {
			t.Fatalf("查询死信失败: %v", err)
		}
		if len(dlqRows) != 1 || dlqRows[0].EventOutboxID != 1 || dlqRows[0].ErrorMsg != "boom" {
			t.Fatalf("推送失败应写死信: %+v", dlqRows)
		}
	})

	t.Run("推送失败但状态已变化跳过死信", func(t *testing.T) {
		pub := &stubPublisher{err: errors.New("boom")}
		p, gormDB, cleanup := newTestProcessor(t, &syncWorker{}, pub)
		defer cleanup()
		row := model.SysEventOutbox{
			ServiceName: "svc", EventID: "event-1", Topic: "topic", Status: model.OutboxStatusSentSuccess,
		}
		row.ID = 1
		if err := gormDB.Create(&row).Error; err != nil {
			t.Fatalf("初始化发件箱数据失败: %v", err)
		}

		if err := p.push(ctx, row); err == nil {
			t.Fatal("推送失败应返回错误")
		}
		var dlqCount int64
		if err := gormDB.Model(&model.SysEventOutboxDlq{}).Count(&dlqCount).Error; err != nil {
			t.Fatalf("查询死信失败: %v", err)
		}
		if dlqCount != 0 {
			t.Fatalf("状态已变化不应写死信，实际 %d 条", dlqCount)
		}
	})
}

// TestProcessorMarkSentSuccessWithRetry 验证回写重试的短路成功与重试耗尽
func TestProcessorMarkSentSuccessWithRetry(t *testing.T) {
	t.Run("状态已非 FETCHED 跳过回写", func(t *testing.T) {
		p, gormDB, cleanup := newTestProcessor(t, &syncWorker{}, &stubPublisher{})
		defer cleanup()
		row := model.SysEventOutbox{
			ServiceName: "svc", EventID: "event-1", Topic: "topic", Status: model.OutboxStatusPending,
		}
		row.ID = 1
		if err := gormDB.Create(&row).Error; err != nil {
			t.Fatalf("初始化发件箱数据失败: %v", err)
		}
		if !p.markSentSuccessWithRetry(context.Background(), &row, time.Now()) {
			t.Fatal("状态已变化时应返回 true 并跳过回写")
		}
	})

	t.Run("持续失败重试耗尽", func(t *testing.T) {
		p := &Processor{
			cfg:            cfg.Runtime{ServiceName: "svc"},
			outboxRepo:     repo.NewOutboxRepo(newDeadDB(t)),
			kafkaPublisher: &stubPublisher{},
			txExecutor:     passthroughExecutor{},
		}
		row := model.SysEventOutbox{ServiceName: "svc", EventID: "event-1", Topic: "topic", Status: model.OutboxStatusFetched}
		row.ID = 1
		if p.markSentSuccessWithRetry(context.Background(), &row, time.Now()) {
			t.Fatal("数据库不可用时回写应重试耗尽返回 false")
		}
	})
}

// TestProcessorOffer 验证单条提交的推送与拒绝恢复分支
func TestProcessorOffer(t *testing.T) {
	ctx := context.Background()

	t.Run("推送成功", func(t *testing.T) {
		pub := &stubPublisher{}
		p, gormDB, cleanup := newTestProcessor(t, &syncWorker{}, pub)
		defer cleanup()
		row := mustInsertFetched(t, gormDB, 1, "event-1")

		p.Offer(ctx, row)
		var stored model.SysEventOutbox
		if err := gormDB.First(&stored, row.ID).Error; err != nil {
			t.Fatalf("查询发件箱失败: %v", err)
		}
		if stored.Status != model.OutboxStatusSentSuccess {
			t.Fatalf("提交后应推送成功，实际 %s", stored.Status)
		}
	})

	t.Run("队列满恢复 PENDING", func(t *testing.T) {
		pub := &stubPublisher{}
		p, gormDB, cleanup := newTestProcessor(t, &fullWorker{}, pub)
		defer cleanup()
		row := mustInsertFetched(t, gormDB, 1, "event-1")

		p.Offer(ctx, row)
		var stored model.SysEventOutbox
		if err := gormDB.First(&stored, row.ID).Error; err != nil {
			t.Fatalf("查询发件箱失败: %v", err)
		}
		if stored.Status != model.OutboxStatusPending || stored.FetchTime != nil {
			t.Fatalf("队列满应恢复为 PENDING: %+v", stored)
		}
		if pub.calls() != 0 {
			t.Fatalf("队列满不应推送，实际推送 %d 次", pub.calls())
		}
	})

	t.Run("不可恢复错误仅记录", func(t *testing.T) {
		pub := &stubPublisher{}
		p, gormDB, cleanup := newTestProcessor(t, &errWorker{err: errors.New("boom")}, pub)
		defer cleanup()
		row := mustInsertFetched(t, gormDB, 1, "event-1")

		p.Offer(ctx, row)
		var stored model.SysEventOutbox
		if err := gormDB.First(&stored, row.ID).Error; err != nil {
			t.Fatalf("查询发件箱失败: %v", err)
		}
		if stored.Status != model.OutboxStatusFetched || stored.FetchTime == nil {
			t.Fatalf("不可恢复错误不应恢复状态: %+v", stored)
		}
	})
}

// TestProcessorOfferBatch 验证批量提交与被拒事件的统一恢复
func TestProcessorOfferBatch(t *testing.T) {
	ctx := context.Background()

	t.Run("空批次直接返回", func(t *testing.T) {
		p := &Processor{}
		p.OfferBatch(ctx, nil)
	})

	t.Run("全部拒绝批量恢复", func(t *testing.T) {
		pub := &stubPublisher{}
		gate := &gateWorker{}
		p, gormDB, cleanup := newTestProcessor(t, gate, pub)
		defer cleanup()
		row1 := mustInsertFetched(t, gormDB, 1, "event-1")
		row2 := mustInsertFetched(t, gormDB, 2, "event-2")

		p.OfferBatch(ctx, []model.SysEventOutbox{row1, row2})
		if got := gate.rejectedCount(); got != 2 {
			t.Fatalf("期望拒绝 2 次，实际 %d 次", got)
		}
		var stored []model.SysEventOutbox
		if err := gormDB.Order("id ASC").Find(&stored).Error; err != nil {
			t.Fatalf("查询发件箱失败: %v", err)
		}
		for i := range stored {
			if stored[i].Status != model.OutboxStatusPending {
				t.Fatalf("被拒事件应恢复为 PENDING，第 %d 条实际 %s", i, stored[i].Status)
			}
		}
	})

	t.Run("部分成功", func(t *testing.T) {
		pub := &stubPublisher{}
		gate := &gateWorker{accept: 1}
		p, gormDB, cleanup := newTestProcessor(t, gate, pub)
		defer cleanup()
		row1 := mustInsertFetched(t, gormDB, 1, "event-1")
		row2 := mustInsertFetched(t, gormDB, 2, "event-2")
		row3 := mustInsertFetched(t, gormDB, 3, "event-3")

		p.OfferBatch(ctx, []model.SysEventOutbox{row1, row2, row3})
		if pub.calls() != 1 {
			t.Fatalf("期望推送 1 次，实际 %d 次", pub.calls())
		}
		var stored model.SysEventOutbox
		if err := gormDB.First(&stored, 1).Error; err != nil {
			t.Fatalf("查询发件箱失败: %v", err)
		}
		if stored.Status != model.OutboxStatusSentSuccess {
			t.Fatalf("首个事件应推送成功，实际 %s", stored.Status)
		}
		var pendingCount int64
		if err := gormDB.Model(&model.SysEventOutbox{}).Where("status = ?", model.OutboxStatusPending).Count(&pendingCount).Error; err != nil {
			t.Fatalf("查询发件箱失败: %v", err)
		}
		if pendingCount != 2 {
			t.Fatalf("被拒事件应恢复为 PENDING，实际 %d 条", pendingCount)
		}
	})
}

// TestShouldRestorePending 验证线程池可恢复错误的判断
func TestShouldRestorePending(t *testing.T) {
	if shouldRestorePending(nil) {
		t.Fatal("nil 错误不应恢复")
	}
	for _, msg := range []string{
		"worker.event.outbox.processor.queue_full",
		"worker.event.outbox.processor.not_run",
		"worker.event.outbox.processor.stopped",
	} {
		if !shouldRestorePending(errors.New(msg)) {
			t.Fatalf("错误 %q 应判定为可恢复", msg)
		}
	}
	if shouldRestorePending(errors.New("boom")) {
		t.Fatal("普通错误不应判定为可恢复")
	}
}
