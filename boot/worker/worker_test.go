// Package worker 工作池测试，覆盖 channel 与 ants 两种后端的接口契约、满队列、超时、优雅停机等场景
//
// @author zwei
package worker

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestWorkerContract 测试工作池接口契约，覆盖 channel 和 ants 两种后端
func TestWorkerContract(t *testing.T) {
	backends := []struct {
		name    string
		factory func(WorkConfig, context.Context) (WorkerService, error)
	}{
		{
			name: "channel",
			factory: func(cfg WorkConfig, parentCtx context.Context) (WorkerService, error) {
				return newChannelWorker(cfg, parentCtx)
			},
		},
		{
			name: "ants",
			factory: func(cfg WorkConfig, parentCtx context.Context) (WorkerService, error) {
				return NewAntsWorker(cfg, parentCtx)
			},
		},
	}

	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			factory := backend.factory
			t.Run("submit and close", func(t *testing.T) {
				testSubmitAndClose(t, factory)
			})
			t.Run("try submit queue full", func(t *testing.T) {
				if backend.name == "channel" {
					testChannelTrySubmitQueueFull(t, factory)
					return
				}
				testAntsTrySubmitQueueFull(t, factory)
			})
			t.Run("submit context cancel", func(t *testing.T) {
				testSubmitContextCancel(t, factory)
			})
			t.Run("close while submit blocked", func(t *testing.T) {
				testCloseWhileSubmitBlocked(t, factory)
			})
			if backend.name == "ants" {
				t.Run("try submit queue full when worker busy", func(t *testing.T) {
					testAntsTrySubmitQueueFullWhenWorkerBusy(t, factory)
				})
				t.Run("close after try submit rejected", func(t *testing.T) {
					testAntsCloseAfterTrySubmitRejected(t, factory)
				})
				t.Run("submit timeout no ghost task", func(t *testing.T) {
					testAntsSubmitTimeoutNoGhostTask(t, factory)
				})
			}
		})
	}
}

// testSubmitAndClose 测试提交任务后关闭工作池，验证任务执行完毕且 Wait 正常返回
func testSubmitAndClose(t *testing.T, factory func(WorkConfig, context.Context) (WorkerService, error)) {
	t.Helper()

	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := factory(WorkConfig{
		Name:    "test",
		WorkNum: 1,
		Size:    4,
	}, parentCtx)
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	pool.Start()

	var done atomic.Bool
	started := make(chan struct{})
	waitRelease := make(chan struct{})
	release := make(chan struct{})

	if err := pool.Submit(func(ctx context.Context) error {
		close(started)
		<-release
		done.Store(true)
		close(waitRelease)
		return nil
	}); err != nil {
		t.Fatalf("submit: %v", err)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("task did not start")
	}

	if pending := pool.Pending(); pending < 0 {
		t.Fatalf("pending: %d", pending)
	}

	close(release)
	select {
	case <-waitRelease:
	case <-time.After(time.Second):
		t.Fatal("task did not finish before close")
	}

	if err := pool.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	pool.Wait()

	if !done.Load() {
		t.Fatal("task did not finish")
	}
}

// testChannelTrySubmitQueueFull 测试 channel 后端满队列时 TrySubmit 立即返回错误
func testChannelTrySubmitQueueFull(t *testing.T, factory func(WorkConfig, context.Context) (WorkerService, error)) {
	t.Helper()

	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := factory(WorkConfig{
		Name:    "test",
		WorkNum: 1,
		Size:    1,
	}, parentCtx)
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	pool.Start()
	defer func() {
		_ = pool.Close()
		pool.Wait()
	}()

	started := make(chan struct{})
	block := make(chan struct{})
	if err := pool.Submit(func(ctx context.Context) error {
		close(started)
		<-block
		return nil
	}); err != nil {
		t.Fatalf("submit blocking task: %v", err)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("blocking task did not start")
	}

	if err := pool.Submit(func(ctx context.Context) error { return nil }); err != nil {
		t.Fatalf("submit queued task: %v", err)
	}

	waitUntil(t, func() bool {
		return pool.Pending() >= 1
	})

	if err := pool.TrySubmit(func(ctx context.Context) error { return nil }); err == nil {
		t.Fatal("expected queue full error")
	} else if err.Error() != "worker.test.queue_full" {
		t.Fatalf("try submit: %v", err)
	}

	close(block)
}

// testAntsTrySubmitQueueFull 测试 ants 后端满队列时 TrySubmit 立即返回错误
func testAntsTrySubmitQueueFull(t *testing.T, factory func(WorkConfig, context.Context) (WorkerService, error)) {
	t.Helper()

	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := factory(WorkConfig{
		Name:    "test",
		WorkNum: 1,
		Size:    1,
	}, parentCtx)
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	pool.Start()
	defer func() {
		_ = pool.Close()
		pool.Wait()
	}()

	started := make(chan struct{})
	block := make(chan struct{})
	if err := pool.Submit(func(ctx context.Context) error {
		close(started)
		<-block
		return nil
	}); err != nil {
		t.Fatalf("submit blocking task: %v", err)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("blocking task did not start")
	}

	go func() {
		_ = pool.Submit(func(ctx context.Context) error {
			<-block
			return nil
		})
	}()

	waitUntil(t, func() bool {
		return pool.Pending() >= 1
	})

	if err := pool.TrySubmit(func(ctx context.Context) error { return nil }); err == nil {
		t.Fatal("expected queue full error")
	} else if err.Error() != "worker.test.queue_full" {
		t.Fatalf("try submit: %v", err)
	}

	close(block)
}

// testSubmitContextCancel 测试提交任务时 ctx 已取消则立即返回错误
func testSubmitContextCancel(t *testing.T, factory func(WorkConfig, context.Context) (WorkerService, error)) {
	t.Helper()

	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := factory(WorkConfig{
		Name:          "test",
		WorkNum:       1,
		Size:          1,
		SubmitTimeout: time.Second,
	}, parentCtx)
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	pool.Start()
	defer func() {
		_ = pool.Close()
		pool.Wait()
	}()

	block := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)

	if err := pool.Submit(func(ctx context.Context) error {
		defer wg.Done()
		<-block
		return nil
	}); err != nil {
		t.Fatalf("submit blocking task: %v", err)
	}

	submitCtx, submitCancel := context.WithCancel(context.Background())
	submitCancel()

	if err := pool.SubmitContext(submitCtx, func(ctx context.Context) error { return nil }); err == nil {
		t.Fatal("expected submit context error")
	}

	close(block)
	wg.Wait()
}

// waitUntil 轮询等待条件满足，超时 1s 则终止测试
func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// testAntsTrySubmitQueueFullWhenWorkerBusy 测试 ants 后端 worker 全忙时 TrySubmit 立即返回 queue_full（即使阻塞队列有空位）
func testAntsTrySubmitQueueFullWhenWorkerBusy(t *testing.T, factory func(WorkConfig, context.Context) (WorkerService, error)) {
	t.Helper()

	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := factory(WorkConfig{Name: "test", WorkNum: 1, Size: 2}, parentCtx)
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	pool.Start()
	defer func() {
		_ = pool.Close()
		pool.Wait()
	}()

	block := make(chan struct{})
	started := make(chan struct{})
	if err := pool.Submit(func(ctx context.Context) error {
		close(started)
		<-block
		return nil
	}); err != nil {
		t.Fatalf("submit blocking task: %v", err)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("blocking task did not start")
	}

	if err := pool.TrySubmit(func(ctx context.Context) error { return nil }); err == nil {
		t.Fatal("expected queue full error when worker is busy")
	} else if err.Error() != "worker.test.queue_full" {
		t.Fatalf("try submit: %v", err)
	}

	close(block)
}

// testAntsCloseAfterTrySubmitRejected 测试 worker 全忙时 TrySubmit 被拒绝后 Close 不会死锁
func testAntsCloseAfterTrySubmitRejected(t *testing.T, factory func(WorkConfig, context.Context) (WorkerService, error)) {
	t.Helper()

	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := factory(WorkConfig{Name: "test", WorkNum: 1, Size: 2}, parentCtx)
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	pool.Start()

	block := make(chan struct{})
	started := make(chan struct{})
	if err := pool.Submit(func(ctx context.Context) error {
		close(started)
		<-block
		return nil
	}); err != nil {
		t.Fatalf("submit blocking task: %v", err)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("blocking task did not start")
	}

	if err := pool.TrySubmit(func(ctx context.Context) error { return nil }); err == nil {
		t.Fatal("expected queue full error when worker is busy")
	}

	done := make(chan struct{})
	go func() {
		_ = pool.Close()
		pool.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		close(block)
		t.Fatal("close deadlocked after rejected try submit")
	}

	close(block)
}

// testAntsSubmitTimeoutNoGhostTask 测试 ants 提交超时返回后不应有幽灵任务继续执行
func testAntsSubmitTimeoutNoGhostTask(t *testing.T, factory func(WorkConfig, context.Context) (WorkerService, error)) {
	t.Helper()

	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := factory(WorkConfig{
		Name:          "test",
		WorkNum:       1,
		Size:          2,
		SubmitTimeout: 50 * time.Millisecond,
	}, parentCtx)
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	pool.Start()
	defer func() {
		_ = pool.Close()
		pool.Wait()
	}()

	block := make(chan struct{})
	started := make(chan struct{})
	if err := pool.Submit(func(ctx context.Context) error {
		close(started)
		<-block
		return nil
	}); err != nil {
		t.Fatalf("submit blocking task: %v", err)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("blocking task did not start")
	}

	var ran atomic.Bool
	if err := pool.Submit(func(ctx context.Context) error {
		ran.Store(true)
		return nil
	}); err == nil {
		t.Fatal("expected submit timeout")
	}

	close(block)
	time.Sleep(100 * time.Millisecond)
	if ran.Load() {
		t.Fatal("task ran after submit returned timeout error")
	}
}

// testCloseWhileSubmitBlocked 测试提交阻塞在入队阶段时 Close 能及时解除阻塞
func testCloseWhileSubmitBlocked(t *testing.T, factory func(WorkConfig, context.Context) (WorkerService, error)) {
	t.Helper()

	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := factory(WorkConfig{
		Name:          "test",
		WorkNum:       1,
		Size:          1,
		SubmitTimeout: 5 * time.Second,
	}, parentCtx)
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	pool.Start()

	block := make(chan struct{})
	started := make(chan struct{})
	if err := pool.Submit(func(ctx context.Context) error {
		close(started)
		<-block
		return nil
	}); err != nil {
		t.Fatalf("submit blocking task: %v", err)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("blocking task did not start")
	}

	// 占满队列，使后续 Submit 阻塞在入队阶段
	if isAntsWorker(pool) {
		go func() {
			_ = pool.Submit(func(ctx context.Context) error {
				<-block
				return nil
			})
		}()
		waitUntil(t, func() bool {
			return pool.Pending() >= 1
		})
	} else {
		if err := pool.Submit(func(ctx context.Context) error { return nil }); err != nil {
			t.Fatalf("submit queued task: %v", err)
		}
	}

	submitDone := make(chan error, 1)
	go func() {
		submitDone <- pool.Submit(func(ctx context.Context) error { return nil })
	}()

	time.Sleep(50 * time.Millisecond)

	go func() {
		_ = pool.Close()
	}()

	select {
	case err := <-submitDone:
		if err == nil {
			t.Fatal("expected submit error after close")
		}
	case <-time.After(500 * time.Millisecond):
		close(block)
		t.Fatal("blocked submit did not return promptly after close")
	}

	close(block)
	pool.Wait()
}

// isAntsWorker 判断工作池是否为 ants 后端实现
func isAntsWorker(pool WorkerService) bool {
	_, ok := pool.(*antsWorker)
	return ok
}

// TestNewWorkerAcceptsNilParentCtx 测试 NewWorker 接受 nil parentCtx，内部归一化为 Background
func TestNewWorkerAcceptsNilParentCtx(t *testing.T) {
	pool, err := NewWorker(WorkConfig{Name: "test", WorkNum: 1, Size: 1}, nil)
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	pool.Start()
	if err := pool.Submit(func(ctx context.Context) error { return nil }); err != nil {
		t.Fatalf("submit: %v", err)
	}
	_ = pool.Close()
	pool.Wait()
}

// TestNewWorkerUsesChannelBackend 测试 NewWorker 默认使用 channel 后端
func TestNewWorkerUsesChannelBackend(t *testing.T) {
	parentCtx := context.Background()
	pool, err := NewWorker(WorkConfig{Name: "test", WorkNum: 1, Size: 1}, parentCtx)
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	if _, ok := pool.(*channelWorker); !ok {
		t.Fatalf("expected channel worker, got %T", pool)
	}
}
