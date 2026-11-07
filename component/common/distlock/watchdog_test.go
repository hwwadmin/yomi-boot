package distlock

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hwwadmin/yomi-boot/component/common/redis/redistest"
	"github.com/hwwadmin/yomi-boot/component/core/errors"

	redisv9 "github.com/redis/go-redis/v9"
)

// countEvalHook 统计成功的 EVAL / EVALSHA，用来确认看门狗确实发出了续期。
type countEvalHook struct {
	n *atomic.Int32
}

// DialHook 实现 DialHook（测试桩 hook）
func (h *countEvalHook) DialHook(next redisv9.DialHook) redisv9.DialHook { return next }

// ProcessHook 在续期脚本成功后计数（测试桩 hook）
func (h *countEvalHook) ProcessHook(next redisv9.ProcessHook) redisv9.ProcessHook {
	return func(ctx context.Context, cmd redisv9.Cmder) error {
		err := next(ctx, cmd)
		if err != nil {
			return err
		}
		switch cmd.Name() {
		case "eval", "evalsha":
			h.n.Add(1)
		}
		return nil
	}
}

// ProcessPipelineHook 实现 ProcessPipelineHook（测试桩 hook）
func (h *countEvalHook) ProcessPipelineHook(next redisv9.ProcessPipelineHook) redisv9.ProcessPipelineHook {
	return next
}

// TestWatchdog_KeepsLockBeyondTTL 验证看门狗续期使锁在 TTL 外仍被持有。
// 每次确认出现一次新的续期后再等待不到一个 TTL，累计等待超过 TTL。不续期时 Key 会被删掉。
func TestWatchdog_KeepsLockBeyondTTL(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	var extends atomic.Int32
	client.AddHook(&countEvalHook{n: &extends})

	factory, err := NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}
	ctx := context.Background()
	key := "test:watchdog-keep"
	ttl := 600 * time.Millisecond

	holder := factory.GetLock(key,
		WithTTL(ttl),
		WithWatchdog(true),
		WithWatchdogInterval(80*time.Millisecond),
	)
	waiter := factory.GetLock(key, WithTTL(ttl))

	if err := holder.Lock(ctx); err != nil {
		t.Fatalf("Lock 失败: %v", err)
	}
	defer func() { _ = holder.Unlock(ctx) }()

	waitExtend := func(after int32) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for extends.Load() <= after {
			if time.Now().After(deadline) {
				t.Fatalf("等待看门狗续期超过 %d 次超时，实际 %d", after, extends.Load())
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	step := ttl / 3
	var waited time.Duration
	for waited < 2*ttl {
		seen := extends.Load()
		waitExtend(seen)
		time.Sleep(step)
		waited += step

		ok, tryErr := waiter.TryLock(ctx)
		if tryErr != nil || ok {
			t.Fatalf("累计等待 %s 后看门狗仍应持有锁，实际 ok=%v err=%v extends=%d",
				waited, ok, tryErr, extends.Load())
		}
	}
	if extends.Load() < 2 {
		t.Fatalf("应观察到多次续期，实际 %d", extends.Load())
	}
}

// TestWatchdog_DisabledLockExpires 验证未启用看门狗时锁会过期
func TestWatchdog_DisabledLockExpires(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}
	ctx := context.Background()
	key := "test:watchdog-off"
	ttl := 800 * time.Millisecond

	holder := factory.GetLock(key, WithTTL(ttl))
	waiter := factory.GetLock(key, WithTTL(ttl))

	if err := holder.Lock(ctx); err != nil {
		t.Fatalf("Lock 失败: %v", err)
	}

	time.Sleep(2 * ttl)

	ok, err := waiter.TryLock(ctx)
	if err != nil || !ok {
		t.Fatalf("无看门狗时期望 true,nil，实际 %v,%v", ok, err)
	}
	if err := waiter.Unlock(ctx); err != nil {
		t.Fatalf("Unlock 失败: %v", err)
	}
	if err := holder.Unlock(ctx); !errors.Is(err, ErrLockLost) {
		t.Fatalf("过期且锁已被他人获取后 holder Unlock 期望 ErrLockLost，实际 %v", err)
	}
	if err := holder.Unlock(ctx); !errors.Is(err, ErrLockNotHeld) {
		t.Fatalf("清理后再次 Unlock 期望 ErrLockNotHeld，实际 %v", err)
	}
	if err := holder.Lock(ctx); err != nil {
		t.Fatalf("状态清理后 holder 重新 Lock 失败: %v", err)
	}
	if err := holder.Unlock(ctx); err != nil {
		t.Fatalf("holder 重新 Unlock 失败: %v", err)
	}
}

// TestWatchdog_MaxHoldDuration 验证 MaxHoldDuration 到期后 HeldContext cancel
func TestWatchdog_MaxHoldDuration(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}
	ctx := context.Background()
	key := "test:watchdog-max-hold"

	lock := factory.GetLock(key,
		WithTTL(5*time.Second),
		WithWatchdog(true),
		WithWatchdogInterval(100*time.Millisecond),
		WithMaxHoldDuration(300*time.Millisecond),
	)

	if err := lock.Lock(ctx); err != nil {
		t.Fatalf("Lock 失败: %v", err)
	}
	defer func() { _ = lock.Unlock(ctx) }()

	held := lock.HeldContext()
	if held == nil {
		t.Fatal("启用看门狗时期望非 nil HeldContext")
	}

	select {
	case <-held.Done():
	case <-time.After(time.Second):
		t.Fatal("MaxHoldDuration 到期后 HeldContext 未 cancel")
	}
}

// TestWatchdog_UnlockStopsGoroutine 验证 Unlock 后看门狗 goroutine 被回收
func TestWatchdog_UnlockStopsGoroutine(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}
	ctx := context.Background()
	key := "test:watchdog-stop"

	before := runtime.NumGoroutine()
	for range 5 {
		lock := factory.GetLock(key,
			WithTTL(time.Second),
			WithWatchdog(true),
			WithWatchdogInterval(50*time.Millisecond),
		)
		if err := lock.Lock(ctx); err != nil {
			t.Fatalf("Lock 失败: %v", err)
		}
		if lock.HeldContext() == nil {
			t.Fatal("HeldContext 期望非 nil")
		}
		if err := lock.Unlock(ctx); err != nil {
			t.Fatalf("Unlock 失败: %v", err)
		}
		if lock.HeldContext() != nil {
			t.Fatal("Unlock 后 HeldContext 期望 nil")
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= before+2 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("Unlock 后 goroutine 未回收，before=%d after=%d", before, runtime.NumGoroutine())
}

// TestWatchdog_LostAfterKeyDeleted 验证 Redis Key 被删后 HeldContext cancel
func TestWatchdog_LostAfterKeyDeleted(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}
	ctx := context.Background()
	key := "test:watchdog-lost"

	lock := factory.GetLock(key,
		WithTTL(time.Second),
		WithWatchdog(true),
		WithWatchdogInterval(100*time.Millisecond),
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
		if held.Err() != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("删除 Redis Key 后 HeldContext 未 cancel")
}

// TestRunWithLock_Success 验证 RunWithLock 正常执行并释放锁
func TestRunWithLock_Success(t *testing.T) {
	lock, cleanup := newTestLock(t, "test:run-with-lock", WithTTL(5*time.Second))
	defer cleanup()

	called := false
	err := RunWithLock(context.Background(), lock, func(ctx context.Context) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("RunWithLock 失败: %v", err)
	}
	if !called {
		t.Fatal("fn 未被调用")
	}
	if err := lock.Unlock(context.Background()); !errors.Is(err, ErrLockNotHeld) {
		t.Fatalf("RunWithLock 后应已释放锁，实际 %v", err)
	}
}

// TestRunWithLock_LockLost 验证 MaxHoldDuration 到期后 RunWithLock 返回 ErrLockLost
func TestRunWithLock_LockLost(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}
	lock := factory.GetLock("test:run-with-lock-lost",
		WithTTL(5*time.Second),
		WithWatchdog(true),
		WithWatchdogInterval(50*time.Millisecond),
		WithMaxHoldDuration(200*time.Millisecond),
	)

	err = RunWithLock(context.Background(), lock, func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
			t.Fatal("HeldContext 未在 MaxHoldDuration 内 cancel")
			return nil
		}
	})
	if !errors.Is(err, ErrLockLost) {
		t.Fatalf("锁丢失时期望 ErrLockLost，实际 %v", err)
	}
}

// TestRunWithLock_UserContextCanceled 验证 Bug 2 修复：
// 启用看门狗时，fn 收到的 ctx 应派生自用户 ctx，
// 用户 ctx 取消时 fn 能立即感知，而不是只能等 HeldContext cancel。
// 修复前 runCtx = held 直接替换用户 ctx，fn 无法被用户取消。
func TestRunWithLock_UserContextCanceled(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}
	// 启用看门狗且不设 MaxHoldDuration，确保 HeldContext 不会自动 cancel
	lock := factory.GetLock("test:run-with-lock-ctx-cancel",
		WithTTL(5*time.Second),
		WithWatchdog(true),
		WithWatchdogInterval(100*time.Millisecond),
	)

	userCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fnDone := make(chan error, 1)
	go func() {
		fnDone <- RunWithLock(userCtx, lock, func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		})
	}()

	// 给 fn 一点时间进入阻塞
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-fnDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("用户 ctx 取消后 fn 期望返回 context.Canceled，实际 %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("用户 ctx 取消后 fn 未在 2s 内返回，可能未感知到用户 ctx 取消")
	}
}

// TestRunWithLock_DeferPropagatesUnlockErrLockLost 验证 fn 正常返回且 held 仍有效时，
// defer Unlock 若检测到 lost（如 stopWatchdog 窗口内 markLost）应向上返回 ErrLockLost。
func TestRunWithLock_DeferPropagatesUnlockErrLockLost(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}
	ctx := context.Background()
	lock := factory.GetLock("test:run-defer-lost",
		WithTTL(5*time.Second),
		WithWatchdog(true),
		WithWatchdogInterval(time.Hour),
	)

	err = RunWithLock(ctx, lock, func(runCtx context.Context) error {
		rl := lock.(*redsyncLock)
		close(rl.watchdogStop)
		<-rl.watchdogDone
		rl.watchdogStop = make(chan struct{})
		rl.watchdogDone = make(chan struct{})
		go func() {
			defer close(rl.watchdogDone)
			<-rl.watchdogStop
			rl.mu.Lock()
			if rl.mutex != nil {
				rl.markLostLocked()
			}
			rl.mu.Unlock()
		}()
		return nil
	})
	if !errors.Is(err, ErrLockLost) {
		t.Fatalf("defer Unlock 检测到 lost 时期望 ErrLockLost，实际 %v", err)
	}
}

// TestRunWithLock_FnErrorAndLockLost 验证 fn 返回错误时，defer Unlock 检测到锁丢失
// 也应将 ErrLockLost 与 fn 错误合并返回，让调用方知晓互斥保护已失效。
func TestRunWithLock_FnErrorAndLockLost(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}
	ctx := context.Background()
	lock := factory.GetLock("test:run-fn-err-lost",
		WithTTL(5*time.Second),
		WithWatchdog(true),
		WithWatchdogInterval(time.Hour),
	)

	fnErr := errors.New("业务错误")
	err = RunWithLock(ctx, lock, func(runCtx context.Context) error {
		rl := lock.(*redsyncLock)
		close(rl.watchdogStop)
		<-rl.watchdogDone
		rl.watchdogStop = make(chan struct{})
		rl.watchdogDone = make(chan struct{})
		go func() {
			defer close(rl.watchdogDone)
			<-rl.watchdogStop
			rl.mu.Lock()
			if rl.mutex != nil {
				rl.markLostLocked()
			}
			rl.mu.Unlock()
		}()
		return fnErr
	})
	if !errors.Is(err, ErrLockLost) {
		t.Fatalf("期望包含 ErrLockLost，实际 %v", err)
	}
	if !errors.Is(err, fnErr) {
		t.Fatalf("期望包含 fn 错误 %v，实际 %v", fnErr, err)
	}
}

// TestWatchdogInterval_Default 验证 watchdogInterval 默认值
func TestWatchdogInterval_Default(t *testing.T) {
	t.Parallel()

	lock := &redsyncLock{opts: LockOptions{TTL: 30 * time.Second}}
	if got := lock.watchdogInterval(); got != 10*time.Second {
		t.Fatalf("watchdogInterval() = %v, want 10s", got)
	}
}

// TestApplyLockOptions_Watchdog 验证看门狗相关 LockOption 合并
func TestApplyLockOptions_Watchdog(t *testing.T) {
	t.Parallel()

	got := applyLockOptions(defaultLockOptions(), []LockOption{
		WithWatchdog(true),
		WithWatchdogInterval(5 * time.Second),
		WithMaxHoldDuration(time.Minute),
	})
	if !got.Watchdog {
		t.Fatal("Watchdog 期望 true")
	}
	if got.WatchdogInterval != 5*time.Second {
		t.Fatalf("WatchdogInterval = %v, want 5s", got.WatchdogInterval)
	}
	if got.MaxHoldDuration != time.Minute {
		t.Fatalf("MaxHoldDuration = %v, want 1m", got.MaxHoldDuration)
	}
}

// TestApplyLockOptions_WatchdogIntervalCapped 验证 WatchdogInterval 受 TTL 上限约束
func TestApplyLockOptions_WatchdogIntervalCapped(t *testing.T) {
	t.Parallel()

	got := applyLockOptions(defaultLockOptions(), []LockOption{
		WithTTL(2 * time.Second),
		WithWatchdog(true),
		WithWatchdogInterval(5 * time.Second),
	})
	want := 2 * time.Second / 3
	if got.WatchdogInterval != want {
		t.Fatalf("WatchdogInterval = %v, want %v", got.WatchdogInterval, want)
	}
}
