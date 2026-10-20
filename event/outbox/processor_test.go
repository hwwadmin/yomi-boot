// Package outbox 的发件箱抓取与发送容量测试
//
// @author zwei
package outbox

import (
	"context"
	"testing"

	"gitee.com/hq_project/yomi-boot/event/cfg"
	"gitee.com/hq_project/yomi-boot/worker"
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
