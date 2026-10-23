// Package roundrobin 轮询策略测试
//
// @author zwei
package roundrobin

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/distlock"
	"gitee.com/hq_project/yomi-boot/component/common/redis/redistest"
	"gitee.com/hq_project/yomi-boot/component/core/appctx"
)

// TestConfig_validate 校验合法配置通过及各非法字段拒绝
func TestConfig_validate(t *testing.T) {
	valid := Simple("test", 200*time.Millisecond)
	if err := valid.validate(); err != nil {
		t.Fatalf("valid config: %v", err)
	}

	cases := []struct {
		name string
		cfg  Config
	}{
		{
			name: "empty name",
			cfg:  Config{Frequency: time.Second, RandomDelayMin: 100 * time.Millisecond, RandomDelayMax: 200 * time.Millisecond, WakeUpInterval: 100 * time.Millisecond},
		},
		{
			name: "frequency too small",
			cfg:  Config{Name: "x", Frequency: 10 * time.Millisecond, RandomDelayMin: 100 * time.Millisecond, RandomDelayMax: 200 * time.Millisecond, WakeUpInterval: 100 * time.Millisecond},
		},
		{
			name: "random min too small",
			cfg:  Config{Name: "x", Frequency: time.Second, RandomDelayMin: 10 * time.Millisecond, RandomDelayMax: 200 * time.Millisecond, WakeUpInterval: 100 * time.Millisecond},
		},
		{
			name: "random max not greater than min",
			cfg:  Config{Name: "x", Frequency: time.Second, RandomDelayMin: 200 * time.Millisecond, RandomDelayMax: 200 * time.Millisecond, WakeUpInterval: 100 * time.Millisecond},
		},
		{
			name: "wake up too small",
			cfg:  Config{Name: "x", Frequency: time.Second, RandomDelayMin: 100 * time.Millisecond, RandomDelayMax: 200 * time.Millisecond, WakeUpInterval: 10 * time.Millisecond},
		},
		{
			name: "negative max run duration",
			cfg:  Config{Name: "x", Frequency: time.Second, RandomDelayMin: 100 * time.Millisecond, RandomDelayMax: 200 * time.Millisecond, WakeUpInterval: 100 * time.Millisecond, MaxRunDuration: -time.Second},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cfg.validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

// TestSimple 校验 Simple 工厂方法的默认延迟与唤醒间隔
func TestSimple(t *testing.T) {
	cfg := Simple("fetch", 500*time.Millisecond)
	if cfg.Name != "fetch" {
		t.Fatalf("name: %s", cfg.Name)
	}
	if cfg.Frequency != 500*time.Millisecond {
		t.Fatalf("frequency: %s", cfg.Frequency)
	}
	if cfg.RandomDelayMin != 100*time.Millisecond {
		t.Fatalf("random min: %s", cfg.RandomDelayMin)
	}
	if cfg.RandomDelayMax != 600*time.Millisecond {
		t.Fatalf("random max: %s", cfg.RandomDelayMax)
	}
	if cfg.WakeUpInterval != 250*time.Millisecond {
		t.Fatalf("wake up: %s", cfg.WakeUpInterval)
	}
}

// TestRun_executesTask 校验 Run 在超时前至少成功执行一次 task
func TestRun_executesTask(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}

	var runs atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()

	cfg := Simple("test-run", 50*time.Millisecond)
	err = Run(ctx, cfg, func(context.Context) error {
		runs.Add(1)
		return nil
	}, factory)
	if err != context.DeadlineExceeded && err != context.Canceled {
		t.Fatalf("Run: %v", err)
	}
	if runs.Load() == 0 {
		t.Fatal("task did not run")
	}
}

// TestRun_contextCancel 校验 ctx 取消后 Run 及时退出
func TestRun_contextCancel(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Simple("test-cancel", 50*time.Millisecond), func(context.Context) error {
			return nil
		}, factory)
	}()

	time.Sleep(120 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after cancel")
	}
}

// TestStart_invalidConfig 校验非法配置时 Start 不启动轮询 goroutine
func TestStart_invalidConfig(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}

	var runs atomic.Int32
	if err := Start(context.Background(), Config{}, func(context.Context) error {
		runs.Add(1)
		return nil
	}, factory); err == nil {
		t.Fatal("expected validation error")
	}

	time.Sleep(100 * time.Millisecond)
	if runs.Load() != 0 {
		t.Fatal("invalid config should not start loop")
	}
}

// TestJitterDuration 校验 jitter 结果落在 [Frequency+Min, Frequency+Max) 区间
func TestJitterDuration(t *testing.T) {
	cfg := Simple("jitter", 100*time.Millisecond)
	for i := 0; i < 20; i++ {
		got := jitterDuration(cfg)
		low := cfg.Frequency + cfg.RandomDelayMin
		high := cfg.Frequency + cfg.RandomDelayMax
		if got < low || got >= high {
			t.Fatalf("jitter out of range [%s, %s): %s", low, high, got)
		}
	}
}

// TestTick_unlockAfterPanic 校验 task panic 后执行锁已释放，后续可再次抢锁
func TestTick_unlockAfterPanic(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}

	cfg := Simple("test-unlock-panic", 50*time.Millisecond)
	ctx := context.Background()

	executed, err := tick(ctx, cfg, func(context.Context) error {
		panic("task panic")
	}, factory)
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if !executed {
		t.Fatal("expected tick to execute task")
	}

	execLock := factory.GetLock(
		execLockKey(cfg.Name),
		distlock.WithTTL(defaultExecLockTTL),
		distlock.WithWatchdog(true),
	)
	held, err := execLock.TryLock(ctx)
	if err != nil {
		t.Fatalf("TryLock after panic tick: %v", err)
	}
	if !held {
		t.Fatal("exec lock not released after panic in tick")
	}
	unlockCtx, cancel := appctx.BackgroundWithTimeout(5 * time.Second)
	defer cancel()
	if err := execLock.Unlock(unlockCtx); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
}

// TestRunTaskSafely_recoversPanic 校验 task panic 被 recover，不向外传播
func TestRunTaskSafely_recoversPanic(t *testing.T) {
	var ran atomic.Bool
	runTaskSafely(context.Background(), "test-panic", func(context.Context) error {
		ran.Store(true)
		panic("task panic")
	})
	if !ran.Load() {
		t.Fatal("task should have run despite panic")
	}
}

// TestTick_cancelsTaskOnLockLost 校验 MaxHoldDuration 到期后 Task 的 ctx 被 cancel
func TestTick_cancelsTaskOnLockLost(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}

	testExecLockOpts = []distlock.LockOption{
		distlock.WithTTL(time.Second),
		distlock.WithWatchdog(true),
		distlock.WithWatchdogInterval(50 * time.Millisecond),
		distlock.WithMaxHoldDuration(150 * time.Millisecond),
	}
	defer func() { testExecLockOpts = nil }()

	cfg := Simple("test-tick-lock-lost", 50*time.Millisecond)
	canceled := make(chan struct{})
	executed, err := tick(context.Background(), cfg, func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			close(canceled)
			return ctx.Err()
		case <-time.After(3 * time.Second):
			t.Error("task ctx 未在锁丢失后 cancel")
			return nil
		}
	}, factory)
	if err != nil && !errors.Is(err, distlock.ErrLockLost) {
		t.Fatalf("tick: %v", err)
	}
	if !executed {
		t.Fatal("expected tick to execute task")
	}

	select {
	case <-canceled:
	default:
		t.Fatal("MaxHoldDuration 到期后 Task 未感知锁丢失")
	}
}

// TestTick_maxRunDurationCancelsTask 校验 MaxRunDuration 到期后 Task 的 ctx 被 cancel
func TestTick_maxRunDurationCancelsTask(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}

	cfg := Simple("test-max-run-duration", 50*time.Millisecond)
	cfg.MaxRunDuration = 150 * time.Millisecond
	canceled := make(chan struct{})
	executed, err := tick(context.Background(), cfg, func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			close(canceled)
			return ctx.Err()
		case <-time.After(3 * time.Second):
			t.Error("task ctx 未在 MaxRunDuration 到期后 cancel")
			return nil
		}
	}, factory)
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if !executed {
		t.Fatal("expected tick to execute task")
	}

	select {
	case <-canceled:
	default:
		t.Fatal("MaxRunDuration 到期后 Task 未感知取消")
	}
}

// TestTick_zeroMaxRunDurationOverridesFactoryDefault 校验 MaxRunDuration=0 显式覆盖工厂级 MaxHoldDuration
func TestTick_zeroMaxRunDurationOverridesFactoryDefault(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory, err := distlock.NewFactory(client,
		distlock.WithDefaultLockOptions(
			distlock.WithWatchdogInterval(30*time.Millisecond),
			distlock.WithMaxHoldDuration(100*time.Millisecond),
		),
	)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}

	cfg := Simple("test-zero-max-run-duration", 50*time.Millisecond)
	canceled := make(chan struct{})
	executed, err := tick(context.Background(), cfg, func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			close(canceled)
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
			return nil
		}
	}, factory)
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if !executed {
		t.Fatal("expected tick to execute task")
	}
	select {
	case <-canceled:
		t.Fatal("MaxRunDuration=0 不应受工厂级 MaxHoldDuration 影响")
	default:
	}
}

// TestShouldStopOnTickError 仅调度 ctx 结束时才应停机
func TestShouldStopOnTickError(t *testing.T) {
	alive := context.Background()
	if shouldStopOnTickError(alive, context.DeadlineExceeded) {
		t.Fatal("DeadlineExceeded with alive ctx must not stop scheduler")
	}
	if shouldStopOnTickError(alive, context.Canceled) {
		t.Fatal("Canceled error with alive ctx must not stop scheduler")
	}
	if shouldStopOnTickError(alive, errors.New("redis down")) {
		t.Fatal("generic error with alive ctx must not stop via shouldStop")
	}
	if shouldStopOnTickError(alive, nil) {
		t.Fatal("nil error must not stop")
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if !shouldStopOnTickError(canceled, context.Canceled) {
		t.Fatal("canceled ctx must stop scheduler")
	}
	if !shouldStopOnTickError(canceled, context.DeadlineExceeded) {
		t.Fatal("deadline with canceled ctx must stop scheduler")
	}
	if !shouldStopOnTickError(canceled, errors.New("other")) {
		t.Fatal("any tick error after ctx done must stop")
	}
}

// TestBackoffDuration 校验指数退避时长及 maxBackoff 上限
func TestBackoffDuration(t *testing.T) {
	if BackoffDuration(1) != time.Second {
		t.Fatalf("backoff 1: %s", BackoffDuration(1))
	}
	if BackoffDuration(6) != 32*time.Second {
		t.Fatalf("backoff 6: %s", BackoffDuration(6))
	}
	if BackoffDuration(7) != maxBackoff {
		t.Fatalf("backoff 7: %s", BackoffDuration(7))
	}
	if BackoffDuration(20) != maxBackoff {
		t.Fatalf("backoff cap: %s", BackoffDuration(20))
	}
}
