package distlock

import (
	"context"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/redis/redistest"

	"github.com/alicebob/miniredis/v2"
	redisv9 "github.com/redis/go-redis/v9"
)

// delayHook 对 Extend 相关命令注入延迟，放大看门狗续期窗口，
// 用于稳定复现 Unlock 与看门狗 markLost 路径竞争导致的死锁。
type delayHook struct {
	delay time.Duration
}

// DialHook 实现 DialHook（测试桩 hook）
func (h *delayHook) DialHook(next redisv9.DialHook) redisv9.DialHook {
	return next
}

// ProcessHook 对 Extend 相关命令注入延迟（测试桩 hook）
func (h *delayHook) ProcessHook(next redisv9.ProcessHook) redisv9.ProcessHook {
	return func(ctx context.Context, cmd redisv9.Cmder) error {
		// redsync.Extend 使用 EVAL/EVALSHA；DEL（Unlock）不延迟
		switch cmd.Name() {
		case "eval", "evalsha":
			time.Sleep(h.delay)
		}
		return next(ctx, cmd)
	}
}

// ProcessPipelineHook 实现 ProcessPipelineHook（测试桩 hook）
func (h *delayHook) ProcessPipelineHook(next redisv9.ProcessPipelineHook) redisv9.ProcessPipelineHook {
	return next
}

// TestWatchdog_UnlockDoesNotDeadlockWithMarkLost 复现并验证修复：
// 当看门狗 ExtendContext 失败、刚进入 markLost 路径时调用 Unlock，
// Unlock 持有 l.mu 后 <-watchdogDone，看门狗又阻塞在 l.mu.Lock() 上
// 互相等待 → 死锁。修复前此测试会超时失败。
func TestWatchdog_UnlockDoesNotDeadlockWithMarkLost(t *testing.T) {
	mr := miniredis.RunT(t)

	client := redisv9.NewClient(&redisv9.Options{Addr: mr.Addr()})
	client.AddHook(&delayHook{delay: 30 * time.Millisecond})
	defer func() { _ = client.Close() }()

	factory, err := NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}
	ctx := context.Background()
	key := "test:watchdog-deadlock"

	// 用很短 interval 让看门狗高频触发续期失败路径
	lock := factory.GetLock(key,
		WithTTL(500*time.Millisecond),
		WithWatchdog(true),
		WithWatchdogInterval(20*time.Millisecond),
	)

	if err := lock.Lock(ctx); err != nil {
		t.Fatalf("Lock 失败: %v", err)
	}

	// 删除 Key 让后续 ExtendContext 返回 false,nil，触发 markLost 路径
	if err := client.Del(ctx, lock.Key()).Err(); err != nil {
		t.Fatalf("Del 失败: %v", err)
	}

	// 等待一个 tick 让看门狗 ExtendContext 进入延迟窗口
	time.Sleep(30 * time.Millisecond)

	// 此时看门狗很可能正在 ExtendContext（被 hook 延迟），
	// 完成后它会进入 l.mu.Lock() 标记 lost。并发调用 Unlock 触发竞争。
	done := make(chan error, 1)
	go func() {
		done <- lock.Unlock(ctx)
	}()

	select {
	case err := <-done:
		_ = err // Unlock 返回即可，不死锁
	case <-time.After(3 * time.Second):
		t.Fatal("Unlock 死锁：看门狗 markLost 路径与 stopWatchdogLocked 互相等待")
	}
}

// TestWatchdog_UnlockDuringExtendInFlight 通过高频 Lock/Unlock 循环
// 在看门狗 ExtendContext 进行中调用 Unlock，验证无死锁。
func TestWatchdog_UnlockDuringExtendInFlight(t *testing.T) {
	mr := miniredis.RunT(t)

	client := redisv9.NewClient(&redisv9.Options{Addr: mr.Addr()})
	client.AddHook(&delayHook{delay: 10 * time.Millisecond})
	defer func() { _ = client.Close() }()

	factory, err := NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}
	ctx := context.Background()

	for i := 0; i < 20; i++ {
		key := "test:watchdog-deadlock-loop"
		lock := factory.GetLock(key,
			WithTTL(500*time.Millisecond),
			WithWatchdog(true),
			WithWatchdogInterval(5*time.Millisecond),
		)
		if err := lock.Lock(ctx); err != nil {
			t.Fatalf("第 %d 次 Lock 失败: %v", i, err)
		}

		// 让看门狗续期若干次后再 Unlock
		time.Sleep(20 * time.Millisecond)

		done := make(chan error, 1)
		go func() { done <- lock.Unlock(ctx) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("第 %d 次 Unlock 死锁", i)
		}
	}
}

// TestUnlock_ReturnsErrLockLostWhenLost 验证看门狗标记 lost 后 Unlock 返回 ErrLockLost
func TestUnlock_ReturnsErrLockLostWhenLost(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}
	ctx := context.Background()
	key := "test:unlock-lost"

	lock := factory.GetLock(key,
		WithTTL(time.Second),
		WithWatchdog(true),
		WithWatchdogInterval(50*time.Millisecond),
	)

	if err := lock.Lock(ctx); err != nil {
		t.Fatalf("Lock 失败: %v", err)
	}

	// 删除 Redis Key 触发看门狗续期失败 → markLost
	if err := client.Del(ctx, lock.Key()).Err(); err != nil {
		t.Fatalf("Del 失败: %v", err)
	}

	held := lock.HeldContext()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if held.Err() != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if held.Err() == nil {
		t.Fatal("等待 markLost 超时")
	}

	if err := lock.Unlock(ctx); !errors.Is(err, ErrLockLost) {
		t.Fatalf("lost 状态下 Unlock 期望 ErrLockLost，实际 %v", err)
	}
}

// TestExtendContext_TimeoutBounds 验证续期超时下限随 TTL 缩放，不再固化 1s
func TestExtendContext_TimeoutBounds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ttl  time.Duration
		want time.Duration
	}{
		{name: "TTL=30s 取 5%", ttl: 30 * time.Second, want: 1500 * time.Millisecond},
		{name: "TTL=500ms 取下限 100ms", ttl: 500 * time.Millisecond, want: 100 * time.Millisecond},
		{name: "TTL=100ms 取 TTL/2 上限", ttl: 100 * time.Millisecond, want: 50 * time.Millisecond},
		{name: "TTL=2s 取 100ms", ttl: 2 * time.Second, want: 100 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := extendContext(tt.ttl)
			defer cancel()

			dl, ok := ctx.Deadline()
			if !ok {
				t.Fatal("extendContext 返回的 ctx 无 Deadline")
			}
			// 允许少量调度误差
			got := time.Until(dl)
			if got < tt.want-50*time.Millisecond || got > tt.want+50*time.Millisecond {
				t.Fatalf("ttl=%v 续期超时 = %v, want ~%v", tt.ttl, got, tt.want)
			}
		})
	}
}

// TestWatchdog_UnlockFailsThenRetry 不让 stopWatchdogLocked 在 Unlock 失败时停止看门狗
func TestWatchdog_UnlockFailsThenRetry(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}
	ctx := context.Background()
	key := "test:watchdog-unlock-fail-retry"

	lock := factory.GetLock(key,
		WithTTL(500*time.Millisecond),
		WithWatchdog(true),
		WithWatchdogInterval(50*time.Millisecond),
	)
	if err := lock.Lock(ctx); err != nil {
		t.Fatalf("Lock 失败: %v", err)
	}

	// 用已取消 ctx 触发 UnlockContext 失败
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	if err := lock.Unlock(canceledCtx); err == nil {
		t.Fatal("已取消 ctx 下 Unlock 期望失败")
	}

	// 此时看门狗已被停止（stopWatchdogLocked 在 UnlockContext 前），
	// 但 l.mutex 未置 nil，重试 Unlock 应能完成
	done := make(chan error, 1)
	go func() { done <- lock.Unlock(ctx) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("重试 Unlock 失败: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Unlock 失败后重试死锁：看门狗已被停止但 l.mutex 仍非 nil")
	}
}

// TestUnlock_CapturesLostMarkedDuringStopWatchdog 验证 Bug 1 修复：
// stopWatchdogLocked 在 <-watchdogDone 期间会释放并重新获取 l.mu，
// 此窗口内看门狗 markLost 会将 l.lost 置 true。
// 修复前用缓存 lost 值会丢失此状态返回 nil；修复后用 lost = lost || l.lost 捕获 true 返回 ErrLockLost。
//
// 通过自定义看门狗 goroutine 精确控制 markLost 时机，
// 避免 delayHook + sleep 方案在 CI 上的时序抖动。
func TestUnlock_CapturesLostMarkedDuringStopWatchdog(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}
	ctx := context.Background()
	key := "test:unlock-lost-reread"

	// 真实看门狗 interval 设为 1h，确保不会自然 tick 干扰测试
	rl := factory.GetLock(key,
		WithTTL(5*time.Second),
		WithWatchdog(true),
		WithWatchdogInterval(time.Hour),
	).(*redsyncLock)

	if err := rl.Lock(ctx); err != nil {
		t.Fatalf("Lock 失败: %v", err)
	}

	// 停掉真实看门狗（它正在 select 上等待 watchdogStop 或 ticker.C）
	close(rl.watchdogStop)
	<-rl.watchdogDone
	// 重新创建通道，准备挂载自定义看门狗
	rl.watchdogStop = make(chan struct{})
	rl.watchdogDone = make(chan struct{})

	// 自定义看门狗：收到 watchdogStop 信号后获取 l.mu 调用 markLostLocked，
	// 精确模拟真实看门狗 ExtendContext 失败后的 markLost 路径。
	go func() {
		defer close(rl.watchdogDone)
		<-rl.watchdogStop
		// 此时 stopWatchdogLocked 已释放 l.mu 并在 <-done 等待
		rl.mu.Lock()
		if rl.mutex != nil {
			rl.markLostLocked()
		}
		rl.mu.Unlock()
	}()

	// 调用 Unlock，触发 stopWatchdogLocked 的释放-重持 l.mu 窗口
	err = rl.Unlock(ctx)
	if !errors.Is(err, ErrLockLost) {
		t.Fatalf("期望 ErrLockLost（markLost 在 stopWatchdogLocked 期间执行），实际 %v", err)
	}

	// 再次 Unlock 应返回 ErrLockNotHeld，验证状态被正确清理
	if err := rl.Unlock(ctx); !errors.Is(err, ErrLockNotHeld) {
		t.Fatalf("lost 状态清理后再次 Unlock 期望 ErrLockNotHeld，实际 %v", err)
	}
}
