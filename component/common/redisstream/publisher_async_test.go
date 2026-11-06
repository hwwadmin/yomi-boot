// Package redisstream 异步发布者单元测试
//
// @author zwei
package redisstream

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hwwadmin/yomi-boot/component/common/worker"
)

// stubWorker 以固定错误模拟工作池提交失败
type stubWorker struct {
	err error
}

func (w *stubWorker) Start() {}

func (w *stubWorker) Submit(_ worker.WorkerHandler) error {
	return w.err
}

func (w *stubWorker) SubmitContext(_ context.Context, _ worker.WorkerHandler) error {
	return w.err
}

func (w *stubWorker) TrySubmit(_ worker.WorkerHandler) error {
	return w.err
}

func (w *stubWorker) Pending() int { return 0 }

func (w *stubWorker) Close() error { return nil }

func (w *stubWorker) Wait() {}

// recordingPublisher 记录同步 Add 是否被调用
type recordingPublisher struct {
	addCalls int
	addErr   error
}

func (p *recordingPublisher) Add(_ context.Context, _ string, _ map[string]string) (string, error) {
	p.addCalls++
	return "1-0", p.addErr
}

func (p *recordingPublisher) AddBytes(_ context.Context, _ string, _ []byte) (string, error) {
	return "1-0", nil
}

func (p *recordingPublisher) AddJSON(_ context.Context, _ string, _ any) (string, error) {
	return "1-0", nil
}

func (p *recordingPublisher) AddAsync(_ context.Context, _ string, _ map[string]string) error {
	return nil
}

func (p *recordingPublisher) AddBytesAsync(_ context.Context, _ string, _ []byte) error {
	return nil
}

func (p *recordingPublisher) AddJSONAsync(_ context.Context, _ string, _ any) error {
	return nil
}

func (p *recordingPublisher) Close() error { return nil }

func TestAddAsync_QueueFullReturnsError(t *testing.T) {
	t.Parallel()
	recorder := &recordingPublisher{}
	asyncPub := NewAsyncPublisher(recorder, &stubWorker{err: errors.New("worker.redisstream.publish.queue_full")}, time.Second)

	err := asyncPub.AddAsync(context.Background(), "orders", map[string]string{"k": "v"})
	if err == nil || err.Error() != "worker.redisstream.publish.queue_full" {
		t.Fatalf("AddAsync err = %v, want queue_full", err)
	}
	if recorder.addCalls != 0 {
		t.Fatalf("入队失败不应改为同步发送，calls=%d", recorder.addCalls)
	}
}

func TestAddAsync_SubmitTimeoutReturnsError(t *testing.T) {
	t.Parallel()
	recorder := &recordingPublisher{}
	asyncPub := NewAsyncPublisher(recorder, &stubWorker{err: errors.New("worker.redisstream.publish.submit_timeout")}, time.Second)

	err := asyncPub.AddAsync(context.Background(), "orders", map[string]string{"k": "v"})
	if err == nil || err.Error() != "worker.redisstream.publish.submit_timeout" {
		t.Fatalf("AddAsync err = %v, want submit_timeout", err)
	}
	if recorder.addCalls != 0 {
		t.Fatalf("入队超时不应改为同步发送，calls=%d", recorder.addCalls)
	}
}

func TestAddAsync_CanceledCtxFailsWithoutSyncFallback(t *testing.T) {
	t.Parallel()
	pool, err := worker.NewWorker(worker.WorkConfig{
		Name:    "redisstream.publish.test",
		WorkNum: 1,
		Size:    8,
	}, context.Background())
	if err != nil {
		t.Fatalf("NewWorker: %v", err)
	}
	pool.Start()
	defer func() {
		_ = pool.Close()
		pool.Wait()
	}()

	recorder := &recordingPublisher{}
	asyncPub := NewAsyncPublisher(recorder, pool, time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := asyncPub.AddAsync(ctx, "orders", map[string]string{"k": "v"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if recorder.addCalls != 0 {
		t.Fatalf("canceled ctx should not fall back to sync add, calls=%d", recorder.addCalls)
	}
}

func TestAddAsync_SubmitTimeoutReturnsWithoutSyncSend(t *testing.T) {
	t.Parallel()
	gate := make(chan struct{})
	pool, err := worker.NewWorker(worker.WorkConfig{
		Name:          "redisstream.publish.test",
		WorkNum:       1,
		Size:          1,
		SubmitTimeout: 100 * time.Millisecond,
	}, context.Background())
	if err != nil {
		t.Fatalf("NewWorker: %v", err)
	}
	pool.Start()
	defer func() {
		_ = pool.Close()
		pool.Wait()
	}()
	defer close(gate) // 先于 pool.Close 执行，解除对唯一 worker 的占用

	// 占住唯一 worker，使其无法消费队列
	if err := pool.Submit(func(_ context.Context) error {
		<-gate
		return nil
	}); err != nil {
		t.Fatalf("Submit blocker: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for pool.Pending() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("worker did not pick up blocker job")
		}
		time.Sleep(5 * time.Millisecond)
	}
	// 占满容量为 1 的队列
	if err := pool.Submit(func(_ context.Context) error { return nil }); err != nil {
		t.Fatalf("Submit filler: %v", err)
	}

	recorder := &recordingPublisher{}
	asyncPub := NewAsyncPublisher(recorder, pool, time.Second)

	begin := time.Now()
	err = asyncPub.AddAsync(context.Background(), "orders", map[string]string{"k": "v"})
	if err == nil || !strings.Contains(err.Error(), "submit_timeout") {
		t.Fatalf("AddAsync err = %v, want submit_timeout", err)
	}
	if elapsed := time.Since(begin); elapsed < 100*time.Millisecond {
		t.Fatalf("expected submit wait of at least 100ms, elapsed=%v", elapsed)
	}
	if recorder.addCalls != 0 {
		t.Fatalf("入队超时不应改为同步发送，calls=%d", recorder.addCalls)
	}
}

func TestAddJSONAsync_EnqueueAndCloseDrains(t *testing.T) {
	t.Parallel()
	pool, err := worker.NewWorker(worker.WorkConfig{
		Name:    "redisstream.publish.test",
		WorkNum: 1,
		Size:    8,
	}, context.Background())
	if err != nil {
		t.Fatalf("NewWorker: %v", err)
	}
	pool.Start()

	recorder := &recordingPublisher{}
	asyncPub := NewAsyncPublisher(recorder, pool, time.Second)

	if err := asyncPub.AddJSONAsync(context.Background(), "orders", map[string]int{"n": 1}); err != nil {
		t.Fatalf("AddJSONAsync: %v", err)
	}
	if err := asyncPub.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if recorder.addCalls != 1 {
		t.Fatalf("sync add calls after drain = %d, want 1", recorder.addCalls)
	}
}
