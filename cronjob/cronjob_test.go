// Package cronjob 业务日历定时任务测试
//
// @author zwei
package cronjob

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
	"yomi-boot/distlock"
	"yomi-boot/redis/redistest"
)

// TestJob_validate 校验合法 Job 通过及各非法字段拒绝
func TestJob_validate(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()
	factory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	h := func(context.Context) error { return nil }

	valid := Job{Name: "svc:job", Spec: "0 0 2 * * *"}
	if err := valid.validate(h, factory); err != nil {
		t.Fatalf("valid job: %v", err)
	}

	cases := []struct {
		name    string
		job     Job
		h       Handler
		factory distlock.DistLockFactory
	}{
		{name: "empty name", job: Job{Spec: "0 0 2 * * *"}, h: h, factory: factory},
		{name: "empty spec", job: Job{Name: "x"}, h: h, factory: factory},
		{name: "nil handler", job: Job{Name: "x", Spec: "0 0 2 * * *"}, h: nil, factory: factory},
		{name: "nil factory", job: Job{Name: "x", Spec: "0 0 2 * * *"}, h: h, factory: nil},
		{name: "bad spec", job: Job{Name: "x", Spec: "not-a-cron"}, h: h, factory: factory},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.job.validate(tc.h, tc.factory); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

// TestStart_invalid 校验非法参数时 Start 不启动调度
func TestStart_invalid(t *testing.T) {
	var runs atomic.Int32
	Start(context.Background(), Job{}, func(context.Context) error {
		runs.Add(1)
		return nil
	}, nil)

	time.Sleep(100 * time.Millisecond)
	if runs.Load() != 0 {
		t.Fatal("invalid start should not run handler")
	}
}

// TestStart_executesHandler 校验短周期 Spec 下 Handler 至少执行一次
func TestStart_executesHandler(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()
	factory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}

	var runs atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Start(ctx, Job{Name: "test-exec", Spec: "*/1 * * * * *"}, func(context.Context) error {
		runs.Add(1)
		return nil
	}, factory)

	deadline := time.After(3 * time.Second)
	for runs.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("handler did not run")
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}
	cancel()
	time.Sleep(50 * time.Millisecond)
}

// TestStart_contextCancelStops 校验 cancel 后不再持续触发
func TestStart_contextCancelStops(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()
	factory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}

	var runs atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())

	Start(ctx, Job{Name: "test-cancel", Spec: "*/1 * * * * *"}, func(context.Context) error {
		runs.Add(1)
		return nil
	}, factory)

	deadline := time.After(3 * time.Second)
	for runs.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("handler did not run before cancel")
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}
	cancel()
	time.Sleep(200 * time.Millisecond)
	after := runs.Load()
	time.Sleep(1200 * time.Millisecond)
	if runs.Load() > after+1 {
		// 允许 Stop 等待窗口内至多再完成一次已触发的 fire
		t.Fatalf("expected scheduling to stop, before=%d after=%d", after, runs.Load())
	}
}

// TestFire_skipWhenLockHeld 校验锁被占用时跳过 Handler
func TestFire_skipWhenLockHeld(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()
	factory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}

	name := "test-skip-lock"
	blocker := factory.GetLock(execLockKey(name), distlock.WithTTL(5*time.Second), distlock.WithWatchdog(false))
	held, err := blocker.TryLock(context.Background())
	if err != nil || !held {
		t.Fatalf("blocker TryLock: held=%v err=%v", held, err)
	}
	defer func() { _ = blocker.Unlock(context.Background()) }()

	var runs atomic.Int32
	fire(context.Background(), name, func(context.Context) error {
		runs.Add(1)
		return nil
	}, factory)
	if runs.Load() != 0 {
		t.Fatal("handler should skip when lock held")
	}
}

// TestFire_runsWhenLockFree 校验抢到锁后执行 Handler
func TestFire_runsWhenLockFree(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()
	factory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}

	var runs atomic.Int32
	fire(context.Background(), "test-fire-ok", func(context.Context) error {
		runs.Add(1)
		return nil
	}, factory)
	if runs.Load() != 1 {
		t.Fatalf("runs=%d", runs.Load())
	}
}

// TestFire_handlerErrorDoesNotPanic 校验 Handler 返回 error 不崩溃且锁可再次获取
func TestFire_handlerErrorDoesNotPanic(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()
	factory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}

	fire(context.Background(), "test-fire-err", func(context.Context) error {
		return errors.New("biz fail")
	}, factory)

	var runs atomic.Int32
	fire(context.Background(), "test-fire-err", func(context.Context) error {
		runs.Add(1)
		return nil
	}, factory)
	if runs.Load() != 1 {
		t.Fatal("lock should be releasable after handler error")
	}
}

// TestFire_handlerPanicDoesNotCrash 校验 Handler panic 被 recover 且锁释放
func TestFire_handlerPanicDoesNotCrash(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()
	factory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}

	fire(context.Background(), "test-fire-panic", func(context.Context) error {
		panic("boom")
	}, factory)

	var runs atomic.Int32
	fire(context.Background(), "test-fire-panic", func(context.Context) error {
		runs.Add(1)
		return nil
	}, factory)
	if runs.Load() != 1 {
		t.Fatal("lock should be releasable after panic")
	}
}

// TestFire_cancelsHandlerOnLockLost 校验 MaxHoldDuration 到期后 Handler 的 ctx 被 cancel
func TestFire_cancelsHandlerOnLockLost(t *testing.T) {
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

	canceled := make(chan struct{})
	fire(context.Background(), "test-fire-lock-lost", func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			close(canceled)
			return ctx.Err()
		case <-time.After(3 * time.Second):
			t.Error("handler ctx 未在锁丢失后 cancel")
			return nil
		}
	}, factory)

	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("MaxHoldDuration 到期后 Handler 未感知锁丢失")
	}
}

// TestExecLockKey 校验锁 Key 前缀
func TestExecLockKey(t *testing.T) {
	if got := execLockKey("svc:job"); got != "cronjob:lock:svc:job" {
		t.Fatalf("got %s", got)
	}
}
