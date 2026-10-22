package distlock

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/redis/redistest"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// newTestLock 创建测试用 Lock 实例，返回锁及 Redis 资源清理函数
func newTestLock(t *testing.T, key string, opts ...LockOption) (Lock, func()) {
	t.Helper()

	client, cleanup := redistest.NewClient(t)
	factory, err := NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
		return nil, cleanup
	}
	return factory.GetLock(key, opts...), cleanup
}

// TestLock 验证 Lock 阻塞获取与 Unlock 释放
func TestLock(t *testing.T) {
	lock, cleanup := newTestLock(t, "test:lock", WithTTL(5*time.Second))
	defer cleanup()

	ctx := context.Background()
	if err := lock.Lock(ctx); err != nil {
		t.Fatalf("Lock 失败: %v", err)
	}
	if err := lock.Unlock(ctx); err != nil {
		t.Fatalf("Unlock 失败: %v", err)
	}
}

// TestTryLock 验证 TryLock 在锁空闲与占用时的行为
func TestTryLock(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}
	ctx := context.Background()
	key := "test:try-lock"
	lock1 := factory.GetLock(key, WithTTL(5*time.Second))
	lock2 := factory.GetLock(key, WithTTL(5*time.Second))

	ok, err := lock1.TryLock(ctx)
	if err != nil || !ok {
		t.Fatalf("空闲时 TryLock 期望 true,nil，实际 %v,%v", ok, err)
	}

	ok, err = lock2.TryLock(ctx)
	if err != nil || ok {
		t.Fatalf("占用时 TryLock 期望 false,nil，实际 %v,%v", ok, err)
	}

	if err := lock1.Unlock(ctx); err != nil {
		t.Fatalf("Unlock 失败: %v", err)
	}

	ok, err = lock2.TryLock(ctx)
	if err != nil || !ok {
		t.Fatalf("释放后 TryLock 期望 true,nil，实际 %v,%v", ok, err)
	}
	if err := lock2.Unlock(ctx); err != nil {
		t.Fatalf("Unlock 失败: %v", err)
	}
}

// TestTryLock_ContextCanceledWhileHeld 验证 ctx 已取消且锁被占用时返回 context 错误
func TestTryLock_ContextCanceledWhileHeld(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}
	ctx := context.Background()
	key := "test:try-lock-cancel"
	holder := factory.GetLock(key, WithTTL(5*time.Second))
	waiter := factory.GetLock(key, WithTTL(5*time.Second))

	if err := holder.Lock(ctx); err != nil {
		t.Fatalf("holder Lock 失败: %v", err)
	}
	defer func() { _ = holder.Unlock(ctx) }()

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()

	ok, err := waiter.TryLock(canceledCtx)
	if ok {
		t.Fatalf("ctx 已取消时 TryLock 期望 false，实际 true")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ctx 已取消且锁被占用时期望 context.Canceled，实际 %v", err)
	}
}

// TestTryLockWithTimeout 验证限时获取成功与超时
func TestTryLockWithTimeout(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}
	ctx := context.Background()
	key := "test:try-timeout"
	holder := factory.GetLock(key, WithTTL(5*time.Second))
	waiter := factory.GetLock(key, WithTTL(5*time.Second), WithAcquireRetryDelay(10*time.Millisecond))

	if err := holder.Lock(ctx); err != nil {
		t.Fatalf("holder Lock 失败: %v", err)
	}

	ok, err := waiter.TryLockWithTimeout(ctx, 50*time.Millisecond)
	if err != nil || ok {
		t.Fatalf("占用时 TryLockWithTimeout 期望 false,nil，实际 %v,%v", ok, err)
	}

	if err := holder.Unlock(ctx); err != nil {
		t.Fatalf("holder Unlock 失败: %v", err)
	}

	ok, err = waiter.TryLockWithTimeout(ctx, time.Second)
	if err != nil || !ok {
		t.Fatalf("释放后 TryLockWithTimeout 期望 true,nil，实际 %v,%v", ok, err)
	}
	if err := waiter.Unlock(ctx); err != nil {
		t.Fatalf("waiter Unlock 失败: %v", err)
	}
}

// TestTryLockWithTimeout_ZeroRetryDelay 验证显式零间隔时外层循环不额外等待
func TestTryLockWithTimeout_ZeroRetryDelay(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}
	ctx := context.Background()
	key := "test:try-timeout-zero-delay"
	holder := factory.GetLock(key, WithTTL(5*time.Second))
	waiter := factory.GetLock(key,
		WithTTL(5*time.Second),
		WithAcquireRetryDelay(0),
	)

	if err := holder.Lock(ctx); err != nil {
		t.Fatalf("holder Lock 失败: %v", err)
	}
	defer func() { _ = holder.Unlock(ctx) }()

	timeout := 50 * time.Millisecond
	start := time.Now()
	ok, err := waiter.TryLockWithTimeout(ctx, timeout)
	elapsed := time.Since(start)

	if ok || err != nil {
		t.Fatalf("占用时 TryLockWithTimeout 期望 false,nil，实际 %v,%v", ok, err)
	}
	if elapsed > timeout+100*time.Millisecond {
		t.Fatalf("零间隔重试应在 timeout 附近返回，耗时 %v", elapsed)
	}
}

// TestTryLockWithTimeout_BoundedByTimeout 验证总等待时长由 timeout 约束，而非 AcquireRetryDelay 累积失控
func TestTryLockWithTimeout_BoundedByTimeout(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}
	ctx := context.Background()
	key := "test:try-timeout-bound"
	holder := factory.GetLock(key, WithTTL(5*time.Second))
	waiter := factory.GetLock(key,
		WithTTL(5*time.Second),
		WithAcquireRetryDelay(50*time.Millisecond),
	)

	if err := holder.Lock(ctx); err != nil {
		t.Fatalf("holder Lock 失败: %v", err)
	}
	defer func() { _ = holder.Unlock(ctx) }()

	timeout := 200 * time.Millisecond
	start := time.Now()
	ok, err := waiter.TryLockWithTimeout(ctx, timeout)
	elapsed := time.Since(start)

	if ok || err != nil {
		t.Fatalf("占用时 TryLockWithTimeout 期望 false,nil，实际 %v,%v", ok, err)
	}
	if elapsed > timeout+150*time.Millisecond {
		t.Fatalf("TryLockWithTimeout 应在 timeout 附近返回，耗时 %v", elapsed)
	}
}

// TestTryLockWithTimeout_ParentContextDeadline 验证父 ctx 超时先于 timeout 时返回 ctx 错误
func TestTryLockWithTimeout_ParentContextDeadline(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}
	ctx := context.Background()
	key := "test:try-timeout-parent-deadline"
	holder := factory.GetLock(key, WithTTL(5*time.Second))
	waiter := factory.GetLock(key, WithTTL(5*time.Second), WithAcquireRetryDelay(10*time.Millisecond))

	if err := holder.Lock(ctx); err != nil {
		t.Fatalf("holder Lock 失败: %v", err)
	}
	defer func() { _ = holder.Unlock(ctx) }()

	parentCtx, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()

	ok, err := waiter.TryLockWithTimeout(parentCtx, time.Second)
	if ok {
		t.Fatalf("父 ctx 超时时 TryLockWithTimeout 期望 false，实际 true")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("父 ctx 超时时期望 context.DeadlineExceeded，实际 %v", err)
	}
}

// TestTryLockWithTimeout_ParentContextCanceled 验证父 ctx 取消时返回 context.Canceled
func TestTryLockWithTimeout_ParentContextCanceled(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}
	ctx := context.Background()
	key := "test:try-timeout-parent-cancel"
	holder := factory.GetLock(key, WithTTL(5*time.Second))
	waiter := factory.GetLock(key, WithTTL(5*time.Second), WithAcquireRetryDelay(10*time.Millisecond))

	if err := holder.Lock(ctx); err != nil {
		t.Fatalf("holder Lock 失败: %v", err)
	}
	defer func() { _ = holder.Unlock(ctx) }()

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()

	ok, err := waiter.TryLockWithTimeout(canceledCtx, time.Second)
	if ok {
		t.Fatalf("父 ctx 已取消时 TryLockWithTimeout 期望 false，实际 true")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("父 ctx 已取消时期望 context.Canceled，实际 %v", err)
	}
}

// TestLock_ContextCanceled 验证 ctx 取消时 Lock 返回错误
func TestLock_ContextCanceled(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}
	ctx := context.Background()
	key := "test:lock-cancel"
	holder := factory.GetLock(key, WithTTL(5*time.Second))
	waiter := factory.GetLock(key, WithTTL(5*time.Second), WithAcquireRetryDelay(10*time.Millisecond))

	if err := holder.Lock(ctx); err != nil {
		t.Fatalf("holder Lock 失败: %v", err)
	}
	defer func() { _ = holder.Unlock(ctx) }()

	waitCtx, cancel := context.WithCancel(ctx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- waiter.Lock(waitCtx)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("期望 context.Canceled，实际 %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("等待 Lock 取消超时")
	}
}

// TestUnlock_RetryAfterFailure 验证 Unlock 通信失败后可重试释放
func TestUnlock_RetryAfterFailure(t *testing.T) {
	lock, cleanup := newTestLock(t, "test:unlock-retry", WithTTL(5*time.Second))
	defer cleanup()

	ctx := context.Background()
	if err := lock.Lock(ctx); err != nil {
		t.Fatalf("Lock 失败: %v", err)
	}

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()

	if err := lock.Unlock(canceledCtx); err == nil {
		t.Fatal("已取消 ctx 下 Unlock 期望返回错误")
	}

	if err := lock.Unlock(ctx); err != nil {
		t.Fatalf("重试 Unlock 失败: %v", err)
	}

	if err := lock.Unlock(ctx); !errors.Is(err, ErrLockNotHeld) {
		t.Fatalf("重复 Unlock 期望 ErrLockNotHeld，实际 %v", err)
	}
}

// TestUnlock 验证未持锁时 Unlock 返回 ErrLockNotHeld
func TestUnlock(t *testing.T) {
	lock, cleanup := newTestLock(t, "test:unlock")
	defer cleanup()

	if err := lock.Unlock(context.Background()); !errors.Is(err, ErrLockNotHeld) {
		t.Fatalf("未持锁 Unlock 期望 ErrLockNotHeld，实际 %v", err)
	}
}

// TestLock_AlreadyHeld 验证同一实例重复加锁返回 ErrLockAlreadyHeld
func TestLock_AlreadyHeld(t *testing.T) {
	lock, cleanup := newTestLock(t, "test:already-held", WithTTL(5*time.Second))
	defer cleanup()

	ctx := context.Background()
	if err := lock.Lock(ctx); err != nil {
		t.Fatalf("首次 Lock 失败: %v", err)
	}
	defer func() { _ = lock.Unlock(ctx) }()

	if err := lock.Lock(ctx); !errors.Is(err, ErrLockAlreadyHeld) {
		t.Fatalf("重复 Lock 期望 ErrLockAlreadyHeld，实际 %v", err)
	}
}

// TestTryLock_LostReturnsErrLockLost 验证锁已丢失时再次加锁返回 ErrLockLost
func TestTryLock_LostReturnsErrLockLost(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}
	ctx := context.Background()
	lock := factory.GetLock("test:try-lock-lost",
		WithTTL(time.Second),
		WithWatchdog(true),
		WithWatchdogInterval(50*time.Millisecond),
	)

	if err := lock.Lock(ctx); err != nil {
		t.Fatalf("Lock 失败: %v", err)
	}
	defer func() { _ = lock.Unlock(ctx) }()

	if err := client.Del(ctx, lock.Key()).Err(); err != nil {
		t.Fatalf("Del 失败: %v", err)
	}

	held := lock.HeldContext()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if held != nil && held.Err() != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if held == nil || held.Err() == nil {
		t.Fatal("等待 markLost 超时")
	}

	ok, err := lock.TryLock(ctx)
	if ok || !errors.Is(err, ErrLockLost) {
		t.Fatalf("lost 状态下 TryLock 期望 false,ErrLockLost，实际 %v,%v", ok, err)
	}
}

// TestLock_MutualExclusion 验证多实例对同一 Key 互斥
func TestLock_MutualExclusion(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}
	ctx := context.Background()
	key := "test:mutex"
	var counter atomic.Int32
	var wg sync.WaitGroup

	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lock := factory.GetLock(key, WithTTL(5*time.Second))
			if err := lock.Lock(ctx); err != nil {
				t.Errorf("Lock 失败: %v", err)
				return
			}
			current := counter.Load()
			time.Sleep(5 * time.Millisecond)
			counter.Store(current + 1)
			if err := lock.Unlock(ctx); err != nil {
				t.Errorf("Unlock 失败: %v", err)
			}
		}()
	}

	wg.Wait()
	if counter.Load() != 8 {
		t.Fatalf("counter 期望 8，实际 %d", counter.Load())
	}
}

// TestUnlock_StolenLockReturnsErrLockLost 验证 Key 被其他实例持有后 Unlock 返回 ErrLockLost 并清理状态
func TestUnlock_StolenLockReturnsErrLockLost(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}
	ctx := context.Background()
	key := "test:unlock-stolen"
	lock := factory.GetLock(key, WithTTL(5*time.Second))

	if err := lock.Lock(ctx); err != nil {
		t.Fatalf("Lock 失败: %v", err)
	}
	// 模拟锁过期后被其他实例获取：Key 仍在，但 token 已不属于本实例
	if err := client.Set(ctx, key, "another-owner-token", 5*time.Second).Err(); err != nil {
		t.Fatalf("Set 失败: %v", err)
	}

	if err := lock.Unlock(ctx); !errors.Is(err, ErrLockLost) {
		t.Fatalf("锁被抢后 Unlock 期望 ErrLockLost，实际 %v", err)
	}
	if err := lock.Unlock(ctx); !errors.Is(err, ErrLockNotHeld) {
		t.Fatalf("锁被抢清理后再次 Unlock 期望 ErrLockNotHeld，实际 %v", err)
	}

	// 清掉他人的 Key，确认实例已清理、可重新加锁
	if err := client.Del(ctx, key).Err(); err != nil {
		t.Fatalf("Del 失败: %v", err)
	}
	if err := lock.Lock(ctx); err != nil {
		t.Fatalf("状态清理后重新 Lock 失败: %v", err)
	}
	if err := lock.Unlock(ctx); err != nil {
		t.Fatalf("重新 Unlock 失败: %v", err)
	}
}
