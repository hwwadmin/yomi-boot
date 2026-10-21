package httprecord

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestMemoryHttpRecordService_QueueFullDrops 验证队列满时 Handle 不阻塞并丢弃
func TestMemoryHttpRecordService_QueueFullDrops(t *testing.T) {
	t.Parallel()

	svc := NewMemoryHttpRecordService(1)
	defer svc.Close()

	svc.Handle(context.Background(), &HttpRecord{TraceID: "a", Path: "/a", Method: "GET"})

	done := make(chan struct{})
	go func() {
		svc.Handle(context.Background(), &HttpRecord{TraceID: "b", Path: "/b", Method: "GET"})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Handle blocked when queue is full")
	}
}

// TestStartMemoryRecordConsumer 验证内存队列消费端点能处理入队记录
func TestStartMemoryRecordConsumer(t *testing.T) {
	t.Parallel()

	svc := NewMemoryHttpRecordService(8)
	defer svc.Close()

	var got atomic.Int64
	var wg sync.WaitGroup
	wg.Add(1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := StartMemoryRecordConsumer(ctx, svc, MemoryRecordConsumerConfig{Workers: 2}, func(_ context.Context, record *HttpRecord) RecordHandleResult {
		if record.TraceID == "ok" {
			got.Add(1)
			wg.Done()
		}
		return HandleOK()
	})
	if err != nil {
		t.Fatalf("StartMemoryRecordConsumer() error = %v", err)
	}

	svc.Handle(context.Background(), &HttpRecord{TraceID: "ok", Path: "/ok", Method: "GET"})

	waitDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for memory consumer")
	}

	if got.Load() != 1 {
		t.Fatalf("handled count = %d, want 1", got.Load())
	}
}

// TestStartMemoryRecordConsumer_DuplicateStart 验证重复启动失败
func TestStartMemoryRecordConsumer_DuplicateStart(t *testing.T) {
	t.Parallel()

	svc := NewMemoryHttpRecordService(4)
	defer svc.Close()

	ctx := context.Background()
	handler := func(_ context.Context, _ *HttpRecord) RecordHandleResult {
		return HandleOK()
	}
	if err := StartMemoryRecordConsumer(ctx, svc, MemoryRecordConsumerConfig{Workers: 1}, handler); err != nil {
		t.Fatalf("first StartMemoryRecordConsumer() error = %v", err)
	}
	if err := StartMemoryRecordConsumer(ctx, svc, MemoryRecordConsumerConfig{Workers: 1}, handler); err == nil {
		t.Fatal("second StartMemoryRecordConsumer() error = nil, want already started")
	}
}
