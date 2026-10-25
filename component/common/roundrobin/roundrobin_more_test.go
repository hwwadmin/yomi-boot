// 轮询调度 Start 成功路径、双锁占用与错误退避的补充测试
//
// @author zwei
package roundrobin

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redisv9 "github.com/redis/go-redis/v9"

	"gitee.com/hq_project/yomi-boot/component/common/distlock"
	"gitee.com/hq_project/yomi-boot/component/common/redis/redistest"
)

// TestStart_executesTaskAndStops 验证 Start 启动轮询并在调度 ctx 结束后停止调度
func TestStart_executesTaskAndStops(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}

	var runs atomic.Int32
	ran := make(chan struct{})
	var once sync.Once
	ctx, cancel := context.WithCancel(context.Background())

	if err := Start(ctx, Simple("test-start-run", 50*time.Millisecond), func(context.Context) error {
		n := runs.Add(1)
		if n == 1 {
			once.Do(func() { close(ran) })
		}
		return nil
	}, factory); err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case <-ran:
	case <-time.After(5 * time.Second):
		t.Fatal("task did not run within 5s")
	}

	cancel()
	// 调度 ctx 结束后等待两个轮询周期，任务计数应停止增长
	time.Sleep(150 * time.Millisecond)
	countAfterCancel := runs.Load()
	time.Sleep(200 * time.Millisecond)
	if got := runs.Load(); got != countAfterCancel {
		t.Fatalf("取消后任务不应继续执行，取消时 %d 次，当前 %d 次", countAfterCancel, got)
	}
}

// TestRun_frequencyLockHeldByOther 验证频率锁被其他实例持有时本实例不执行任务
func TestRun_frequencyLockHeldByOther(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}

	const name = "test-freq-hold"
	freqLock := factory.GetLock(frequencyLockKey(name), distlock.WithTTL(5*time.Second), distlock.WithWatchdog(false))
	held, err := freqLock.TryLock(context.Background())
	if err != nil || !held {
		t.Fatalf("预占频率锁失败: held=%v err=%v", held, err)
	}
	defer func() { _ = freqLock.Unlock(context.Background()) }()

	var runs atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()

	err = Run(ctx, Simple(name, 50*time.Millisecond), func(context.Context) error {
		runs.Add(1)
		return nil
	}, factory)
	if err != context.DeadlineExceeded {
		t.Fatalf("Run 应以超时结束，实际 %v", err)
	}
	if runs.Load() != 0 {
		t.Fatal("频率锁被占用时任务不应执行")
	}
}

// TestRun_execLockHeldByOther 验证执行锁被其他实例持有时本实例不执行任务
func TestRun_execLockHeldByOther(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}

	const name = "test-exec-hold"
	execLock := factory.GetLock(execLockKey(name), distlock.WithTTL(5*time.Second), distlock.WithWatchdog(false))
	held, err := execLock.TryLock(context.Background())
	if err != nil || !held {
		t.Fatalf("预占执行锁失败: held=%v err=%v", held, err)
	}
	defer func() { _ = execLock.Unlock(context.Background()) }()

	var runs atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()

	err = Run(ctx, Simple(name, 50*time.Millisecond), func(context.Context) error {
		runs.Add(1)
		return nil
	}, factory)
	if err != context.DeadlineExceeded {
		t.Fatalf("Run 应以超时结束，实际 %v", err)
	}
	if runs.Load() != 0 {
		t.Fatal("执行锁被占用时任务不应执行")
	}
}

// TestRun_tickErrorBacksOffUntilCtxDone 验证 tick 持续失败时按退避重试直至 ctx 结束
func TestRun_tickErrorBacksOffUntilCtxDone(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redisv9.NewClient(&redisv9.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	factory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}

	// 先关闭 Redis：每次 tick 的 TryLock 都失败，进入指数退避
	mr.Close()

	var runs atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 1300*time.Millisecond)
	defer cancel()

	err = Run(ctx, Simple("test-tick-error", 50*time.Millisecond), func(context.Context) error {
		runs.Add(1)
		return nil
	}, factory)
	if err != context.DeadlineExceeded {
		t.Fatalf("Run 应以超时结束，实际 %v", err)
	}
	if runs.Load() != 0 {
		t.Fatal("Redis 不可用时任务不应执行")
	}
}

// fastForwardTicker 持续推进 miniredis 的 TTL 时钟。
// miniredis 不做实时过期，roundrobin 的频率锁依赖 TTL 自然过期让出窗口，
// 多轮轮询的测试须由本协程模拟时间流逝。
func fastForwardTicker(ctx context.Context, mr *miniredis.Miniredis, step time.Duration) {
	ticker := time.NewTicker(step / 2)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mr.FastForward(step)
		}
	}
}

// TestRun_taskErrorContinues 验证任务返回错误不影响后续轮询
func TestRun_taskErrorContinues(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redisv9.NewClient(&redisv9.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	factory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}

	var runs atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go fastForwardTicker(ctx, mr, 200*time.Millisecond)

	err = Run(ctx, Simple("test-task-error", 50*time.Millisecond), func(context.Context) error {
		runs.Add(1)
		return errors.New("task failed")
	}, factory)
	if err != context.DeadlineExceeded {
		t.Fatalf("Run 应以超时结束，实际 %v", err)
	}
	if runs.Load() < 3 {
		t.Fatalf("任务失败后应继续轮询，实际执行 %d 次", runs.Load())
	}
}

// TestRun_successContinuesWithFastForward 验证任务成功后轮询按频率持续执行
func TestRun_successContinuesWithFastForward(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redisv9.NewClient(&redisv9.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	factory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}

	var runs atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go fastForwardTicker(ctx, mr, 200*time.Millisecond)

	err = Run(ctx, Simple("test-success-loop", 50*time.Millisecond), func(context.Context) error {
		runs.Add(1)
		return nil
	}, factory)
	if err != context.DeadlineExceeded {
		t.Fatalf("Run 应以超时结束，实际 %v", err)
	}
	if runs.Load() < 3 {
		t.Fatalf("任务应按频率持续执行，实际执行 %d 次", runs.Load())
	}
}

// TestValidateStartArgsGuards 验证启动入参校验的任务与工厂分支
func TestValidateStartArgsGuards(t *testing.T) {
	cfg := Simple("test-validate", 50*time.Millisecond)
	if err := validateStartArgs(cfg, nil, &fakeFactoryForRR{}); err == nil {
		t.Fatal("task 为空应返回错误")
	}
	if err := validateStartArgs(cfg, func(context.Context) error { return nil }, nil); err == nil {
		t.Fatal("factory 为空应返回错误")
	}
	if err := validateStartArgs(cfg, func(context.Context) error { return nil }, &fakeFactoryForRR{}); err != nil {
		t.Fatalf("合法入参不应报错: %v", err)
	}
}

// fakeFactoryForRR 返回 nil 锁的工厂桩，仅用于入参校验路径
type fakeFactoryForRR struct{}

// GetLock 返回 nil 锁
func (fakeFactoryForRR) GetLock(string, ...distlock.LockOption) distlock.Lock { return nil }
