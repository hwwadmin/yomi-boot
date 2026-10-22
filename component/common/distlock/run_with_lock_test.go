package distlock

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/redis/redistest"
	"gitee.com/hq_project/yomi-boot/core/errors"

	redisv9 "github.com/redis/go-redis/v9"
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
