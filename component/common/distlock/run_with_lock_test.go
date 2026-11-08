package distlock

import (
	"context"
	stderrors "errors"
	"github.com/hwwadmin/yomi-boot/component/common/redis/redistest"
	"github.com/hwwadmin/yomi-boot/component/core/errors"
	redisv9 "github.com/redis/go-redis/v9"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// releaseFailHook 在开关打开时让 EVAL/EVALSHA 失败，用于模拟释放锁时的 Redis 通信错误。
type releaseFailHook struct {
	fail *atomic.Bool // fail 为 true 时释放脚本失败
}

// DialHook 实现 DialHook（测试桩 hook）
func (h *releaseFailHook) DialHook(next redisv9.DialHook) redisv9.DialHook { return next }

// ProcessHook 在开关打开时让释放脚本返回错误（测试桩 hook）
func (h *releaseFailHook) ProcessHook(next redisv9.ProcessHook) redisv9.ProcessHook {
	return func(ctx context.Context, cmd redisv9.Cmder) error {
		switch cmd.Name() {
		case "eval", "evalsha":
			if h.fail.Load() {
				return errors.New("injected release failure")
			}
		}
		return next(ctx, cmd)
	}
}

// ProcessPipelineHook 实现 ProcessPipelineHook（测试桩 hook）
func (h *releaseFailHook) ProcessPipelineHook(next redisv9.ProcessPipelineHook) redisv9.ProcessPipelineHook {
	return next
}

// TestRunWithLock_UnlockFailurePropagated 验证释放锁失败时错误向上传播，而不是静默返回 nil
func TestRunWithLock_UnlockFailurePropagated(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	var fail atomic.Bool
	client.AddHook(&releaseFailHook{fail: &fail})

	factory, err := NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}
	ctx := context.Background()
	lock := factory.GetLock("test:run-unlock-failure", WithTTL(5*time.Second))

	err = RunWithLock(ctx, lock, func(context.Context) error {
		fail.Store(true) // 让随后的 Unlock 失败
		return nil
	})
	if err == nil {
		t.Fatal("释放失败时 RunWithLock 期望返回错误，实际 nil")
	}
	if errors.Is(err, ErrLockLost) {
		t.Fatalf("普通释放失败不应误报 ErrLockLost: %v", err)
	}
	if !strings.Contains(err.Error(), "释放分布式锁失败") {
		t.Fatalf("期望错误包含释放失败原因，实际 %v", err)
	}

	// 关闭故障注入后重试 Unlock，确认实例状态仍可清理
	fail.Store(false)
	if unlockErr := lock.Unlock(ctx); unlockErr != nil {
		t.Fatalf("重试 Unlock 失败: %v", unlockErr)
	}
}

// TestRunWithLock_KeyDeletedReportsLockLost 验证业务执行期间 Key 被删除时 RunWithLock 返回 ErrLockLost
func TestRunWithLock_KeyDeletedReportsLockLost(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}
	ctx := context.Background()
	lock := factory.GetLock("test:run-key-deleted",
		WithTTL(30*time.Second),
		WithWatchdog(true),
		WithWatchdogInterval(time.Hour),
	)

	err = RunWithLock(ctx, lock, func(context.Context) error {
		time.Sleep(50 * time.Millisecond) // 让看门狗首个立即续期完成
		return client.Del(ctx, lock.Key()).Err()
	})
	if !errors.Is(err, ErrLockLost) {
		t.Fatalf("Key 被删后 RunWithLock 期望 ErrLockLost，实际 %v", err)
	}
	if err := lock.Unlock(ctx); !errors.Is(err, ErrLockNotHeld) {
		t.Fatalf("锁丢失后实例状态未清理，再次 Unlock 返回 %v", err)
	}
}

// TestRunWithLock_StolenLockReportsLockLost 验证 Key 被其他实例持有后 RunWithLock 返回 ErrLockLost
func TestRunWithLock_StolenLockReportsLockLost(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}
	ctx := context.Background()
	lock := factory.GetLock("test:run-lock-stolen",
		WithTTL(30*time.Second),
		WithWatchdog(true),
		WithWatchdogInterval(time.Hour),
	)

	err = RunWithLock(ctx, lock, func(context.Context) error {
		time.Sleep(50 * time.Millisecond) // 让看门狗首个立即续期完成
		return client.Set(ctx, lock.Key(), "another-owner-token", 30*time.Second).Err()
	})
	if !errors.Is(err, ErrLockLost) {
		t.Fatalf("锁被抢后 RunWithLock 期望 ErrLockLost，实际 %v", err)
	}
	_ = client.Del(ctx, lock.Key()).Err()
}

func TestRunWithLock_NilAndLockErrorAndLost(t *testing.T) {
	if err := RunWithLock(context.Background(), nil, func(context.Context) error { return nil }); err == nil {
		t.Fatal("nil lock 应失败")
	}
	if err := RunWithLock(context.Background(), &stubHeldLock{}, nil); err == nil {
		t.Fatal("nil fn 应失败")
	}
	if err := RunWithLock(context.Background(), &errLock{}, func(context.Context) error { return nil }); err == nil {
		t.Fatal("加锁失败应返回错误")
	}

	held, cancel := context.WithCancel(context.Background())
	cancel()
	err := RunWithLock(context.Background(), &stubHeldLock{held: held}, func(context.Context) error {
		return stderrors.New("biz")
	})
	if err == nil || !errors.Is(err, ErrLockLost) {
		t.Fatalf("业务失败且锁已丢失时应合并 ErrLockLost: %v", err)
	}
}

type errLock struct{ stubHeldLock }

func (errLock) Lock(context.Context) error { return stderrors.New("lock down") }

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
