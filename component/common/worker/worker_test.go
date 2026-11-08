// Package worker 工作池测试，覆盖 channel 与 ants 两种后端的接口契约、满队列、超时、优雅停机等场景
//
// @author zwei
package worker

import (
	"context"
	"errors"
	"strings"
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

func TestAntsTrySubmit_RunsWhenCapacityFree(t *testing.T) {
	pool, err := NewAntsWorker(WorkConfig{Name: "ants-try", WorkNum: 2, Size: 2}, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	pool.Start()

	if err := pool.TrySubmit(func(context.Context) error { return nil }); err != nil {
		t.Fatalf("有空闲 worker 时 TrySubmit 应成功: %v", err)
	}
}

func TestAntsSubmit_CallerCancelAndStopAndTimeout(t *testing.T) {
	hold := make(chan struct{})
	cfg := WorkConfig{Name: "ants-block", WorkNum: 1, Size: 8, SubmitTimeout: 40 * time.Millisecond}
	pool, err := NewAntsWorker(cfg, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		close(hold)
		_ = pool.Close()
	})
	pool.Start()

	started := make(chan struct{})
	if err := pool.Submit(func(context.Context) error {
		close(started)
		<-hold
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	<-started

	if err := pool.Submit(func(context.Context) error { return nil }); err == nil || !strings.Contains(err.Error(), "submit_timeout") {
		t.Fatalf("工作池占满时应入队超时: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- pool.SubmitContext(ctx, func(context.Context) error { return nil })
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	if err := <-errCh; err == nil {
		t.Fatal("调用方取消后阻塞提交应返回错误")
	}
}

func TestAntsSubmit_StopsWhenWorkerClosed(t *testing.T) {
	hold := make(chan struct{})
	parent, stopParent := context.WithCancel(context.Background())
	cfg := WorkConfig{Name: "ants-stop", WorkNum: 1, Size: 8, SubmitTimeout: time.Second}
	raw, err := newAntsWorker(cfg, parent)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		close(hold)
		stopParent()
		_ = raw.Close()
	})
	raw.Start()

	started := make(chan struct{})
	if err := raw.Submit(func(context.Context) error {
		close(started)
		<-hold
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	<-started

	errCh := make(chan error, 1)
	go func() {
		errCh <- raw.Submit(func(context.Context) error { return nil })
	}()
	time.Sleep(20 * time.Millisecond)
	stopParent()
	if err := <-errCh; err == nil || !strings.Contains(err.Error(), "stopped") {
		t.Fatalf("工作池停止后阻塞提交应失败: %v", err)
	}
}

func TestChannelSubmit_CallerCancelWhileQueued(t *testing.T) {
	hold := make(chan struct{})
	cfg := WorkConfig{Name: "ch-cancel", WorkNum: 1, Size: 1, SubmitTimeout: time.Second}
	pool, err := NewWorker(cfg, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		close(hold)
		_ = pool.Close()
	})
	pool.Start()

	started := make(chan struct{})
	if err := pool.Submit(func(context.Context) error {
		close(started)
		<-hold
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	<-started
	if err := pool.Submit(func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- pool.SubmitContext(ctx, func(context.Context) error { return nil })
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	if err := <-errCh; err == nil {
		t.Fatal("队列已满时取消调用方上下文应返回错误")
	}
}

func TestShutdown_AlreadyClosed(t *testing.T) {
	antsW, err := newAntsWorker(WorkConfig{Name: "ants-closed", WorkNum: 1, Size: 1}, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		antsW.cancel()
		if antsW.pool != nil {
			antsW.pool.Release()
		}
	})
	antsW.status = antsStatusClosed
	antsW.shutdown()

	channelW, err := newChannelWorker(WorkConfig{Name: "ch-closed", WorkNum: 1, Size: 1}, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(channelW.cancel)
	channelW.status = channelStatusClosed
	channelW.shutdown()
}

func TestWatchParentCtx_RecoversPanic(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()

	antsW := &antsWorker{
		cfg:       WorkConfig{Name: "ants-watch"},
		parentCtx: parent,
		cancel:    func() {},
	}
	antsW.watchParentCtx()

	channelW := &channelWorker{
		cfg:       WorkConfig{Name: "ch-watch"},
		parentCtx: parent,
		cancel:    func() {},
		jobs:      make(chan WorkerHandler),
	}
	channelW.watchParentCtx()
}

func TestNewWorker_InvalidConfig(t *testing.T) {
	cases := []WorkConfig{
		{},
		{Name: "pool"},
		{Name: "pool", WorkNum: 1},
		{Name: "pool", WorkNum: 1, Size: 1, SubmitTimeout: -time.Second},
	}
	for _, cfg := range cases {
		if _, err := NewWorker(cfg, nil); err == nil {
			t.Fatalf("非法 channel 配置应失败: %+v", cfg)
		}
		if _, err := NewAntsWorker(cfg, nil); err == nil {
			t.Fatalf("非法 ants 配置应失败: %+v", cfg)
		}
	}
}

func TestWorker_GuardBranches(t *testing.T) {
	cfg := WorkConfig{Name: "guard", WorkNum: 1, Size: 1}
	channelPool, err := NewWorker(cfg, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	antsPool, err := NewAntsWorker(cfg, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = channelPool.Close()
		_ = antsPool.Close()
	})

	if err := channelPool.Submit(nil); err == nil {
		t.Fatal("空 handler 应失败")
	}
	if err := antsPool.Submit(nil); err == nil {
		t.Fatal("ants 空 handler 应失败")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := channelPool.SubmitContext(canceled, func(context.Context) error { return nil }); err == nil {
		t.Fatal("已取消的 ctx 应失败")
	}
	if err := antsPool.SubmitContext(canceled, func(context.Context) error { return nil }); err == nil {
		t.Fatal("ants 已取消的 ctx 应失败")
	}
	if err := channelPool.TrySubmit(func(context.Context) error { return nil }); err == nil {
		t.Fatal("未启动时 TrySubmit 应失败")
	}
	if err := antsPool.TrySubmit(func(context.Context) error { return nil }); err == nil {
		t.Fatal("ants 未启动时 TrySubmit 应失败")
	}

	channelPool.Start()
	channelPool.Start()
	antsPool.Start()
	antsPool.Start()

	if (&antsWorker{}).Pending() != 0 {
		t.Fatal("没有 pool 时 Pending 应为 0")
	}
	stopChannelTimer(nil)
	stopAntsTimer(nil)

	if err := channelPool.Submit(func(context.Context) error { return errors.New("job") }); err != nil {
		t.Fatal(err)
	}
	if err := antsPool.Submit(func(context.Context) error { return errors.New("job") }); err != nil {
		t.Fatal(err)
	}
	if err := channelPool.Submit(func(context.Context) error { panic("channel") }); err != nil {
		t.Fatal(err)
	}
	if err := antsPool.Submit(func(context.Context) error { panic("ants") }); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)

	if err := channelPool.Close(); err != nil {
		t.Fatal(err)
	}
	if err := channelPool.Close(); err != nil {
		t.Fatal(err)
	}
	if err := antsPool.Close(); err != nil {
		t.Fatal(err)
	}
	if err := antsPool.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestWorker_QueueAndDirectConstructors(t *testing.T) {
	if _, err := newChannelWorker(WorkConfig{}, nil); err == nil {
		t.Fatal("channel 构造应校验配置")
	}
	if _, err := newAntsWorker(WorkConfig{}, nil); err == nil {
		t.Fatal("ants 构造应校验配置")
	}

	cfg := WorkConfig{Name: "queue", WorkNum: 1, Size: 1, SubmitTimeout: 30 * time.Millisecond}
	pool, err := NewWorker(cfg, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pool.Close() }()

	if err := pool.SubmitContext(context.Background(), func(context.Context) error { return nil }); err == nil {
		t.Fatal("未启动时提交应失败")
	}
	if err := pool.TrySubmit(nil); err == nil {
		t.Fatal("TrySubmit 空 handler 应失败")
	}

	pool.Start()
	hold := make(chan struct{})
	started := make(chan struct{})
	if err := pool.Submit(func(context.Context) error {
		close(started)
		<-hold
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	<-started
	if err := pool.TrySubmit(func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := pool.TrySubmit(func(context.Context) error { return nil }); err == nil {
		t.Fatal("队列满时 TrySubmit 应失败")
	}
	if err := pool.Submit(func(context.Context) error { return nil }); err == nil {
		t.Fatal("队列满时 Submit 应超时")
	}
	close(hold)
}
