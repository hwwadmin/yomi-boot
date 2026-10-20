// 业务日历定时任务测试
//
// @author zwei

package cronjob

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/distlock"
	"gitee.com/hq_project/yomi-boot/redis/redistest"
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

	validTZ := Job{Name: "svc:job", Spec: "CRON_TZ=UTC 0 0 2 * * *"}
	if err := validTZ.validate(h, factory); err != nil {
		t.Fatalf("valid tz job: %v", err)
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
		// robfig 解析器对 TZ 前缀畸形输入会 panic，validate 必须转为 error
		{name: "tz prefix without expression", job: Job{Name: "x", Spec: "TZ=Asia/Shanghai"}, h: h, factory: factory},
		{name: "cron tz prefix without expression", job: Job{Name: "x", Spec: "CRON_TZ=UTC"}, h: h, factory: factory},
		{name: "empty tz prefix", job: Job{Name: "x", Spec: "TZ="}, h: h, factory: factory},
		{name: "negative max run duration", job: Job{Name: "x", Spec: "0 0 2 * * *", MaxRunDuration: -time.Second}, h: h, factory: factory},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.job.validate(tc.h, tc.factory); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

// TestStart_invalid 校验非法参数时 Start 返回错误与已关闭的 done，且不启动调度
func TestStart_invalid(t *testing.T) {
	var runs atomic.Int32
	done, err := Start(context.Background(), Job{}, func(context.Context) error {
		runs.Add(1)
		return nil
	}, nil)
	if err == nil {
		t.Fatal("invalid start should return error")
	}

	select {
	case <-done:
	default:
		t.Fatal("invalid start should return closed done")
	}
	if runs.Load() != 0 {
		t.Fatal("invalid start should not run handler")
	}
}

// TestMustStart_invalidPanics 校验 MustStart 对非法参数 panic
func TestMustStart_invalidPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("MustStart should panic on invalid job")
		}
	}()
	MustStart(context.Background(), Job{}, func(context.Context) error { return nil }, nil)
}

// TestStart_malformedTZSpecDoesNotPanic 校验畸形 TZ 前缀 Spec 不会让 Start panic
func TestStart_malformedTZSpecDoesNotPanic(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()
	factory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}

	var runs atomic.Int32
	done, err := Start(context.Background(), Job{Name: "test-tz", Spec: "TZ=Asia/Shanghai"}, func(context.Context) error {
		runs.Add(1)
		return nil
	}, factory)
	if err == nil {
		t.Fatal("malformed tz spec should return error")
	}

	select {
	case <-done:
	default:
		t.Fatal("malformed tz spec should return closed done")
	}
	if runs.Load() != 0 {
		t.Fatal("malformed tz spec should not run handler")
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

	done, err := Start(ctx, Job{Name: "test-exec", Spec: "*/1 * * * * *"}, func(context.Context) error {
		runs.Add(1)
		return nil
	}, factory)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
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
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("done did not close after cancel")
	}
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

	if _, err := Start(ctx, Job{Name: "test-cancel", Spec: "*/1 * * * * *"}, func(context.Context) error {
		runs.Add(1)
		return nil
	}, factory); err != nil {
		t.Fatalf("Start: %v", err)
	}

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

// TestStart_doneClosesAfterCancel 校验 cancel 后 Start 返回的 done 关闭
func TestStart_doneClosesAfterCancel(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()
	factory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done, err := Start(ctx, Job{Name: "test-start-done", Spec: "*/1 * * * * *"}, func(context.Context) error {
		return nil
	}, factory)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("done channel did not close after cancel")
	}
}

// TestStart_duplicateNameRejected 校验同一进程内同名 Job 拒绝启动，释放后可重新启动
func TestStart_duplicateNameRejected(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()
	factory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}

	job := Job{Name: "test-duplicate", Spec: "0 0 2 * * *"}
	h := func(context.Context) error { return nil }

	ctx, cancel := context.WithCancel(context.Background())
	done, err := Start(ctx, job, h, factory)
	if err != nil {
		t.Fatalf("first Start: %v", err)
	}

	dupDone, err := Start(ctx, job, h, factory)
	if !errors.Is(err, ErrDuplicateJobName) {
		t.Fatalf("duplicate Start err = %v, want ErrDuplicateJobName", err)
	}
	select {
	case <-dupDone:
	default:
		t.Fatal("duplicate Start should return closed done")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("done did not close after cancel")
	}

	ctx2, cancel2 := context.WithCancel(context.Background())
	done2, err := Start(ctx2, job, h, factory)
	if err != nil {
		t.Fatalf("restart after release: %v", err)
	}
	cancel2()
	select {
	case <-done2:
	case <-time.After(2 * time.Second):
		t.Fatal("done2 did not close after cancel")
	}
}

// TestMustStart_duplicateNamePanics 校验 MustStart 对同名 Job panic
func TestMustStart_duplicateNamePanics(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()
	factory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}

	job := Job{Name: "test-must-duplicate", Spec: "0 0 2 * * *"}
	h := func(context.Context) error { return nil }
	ctx, cancel := context.WithCancel(context.Background())
	done, err := Start(ctx, job, h, factory)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		cancel()
		<-done
	}()

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("MustStart should panic on duplicate name")
		}
	}()
	MustStart(ctx, job, h, factory)
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
	fire(context.Background(), Job{Name: name}, func(context.Context) error {
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
	fire(context.Background(), Job{Name: "test-fire-ok"}, func(context.Context) error {
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

	fire(context.Background(), Job{Name: "test-fire-err"}, func(context.Context) error {
		return errors.New("biz fail")
	}, factory)

	var runs atomic.Int32
	fire(context.Background(), Job{Name: "test-fire-err"}, func(context.Context) error {
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

	fire(context.Background(), Job{Name: "test-fire-panic"}, func(context.Context) error {
		panic("boom")
	}, factory)

	var runs atomic.Int32
	fire(context.Background(), Job{Name: "test-fire-panic"}, func(context.Context) error {
		runs.Add(1)
		return nil
	}, factory)
	if runs.Load() != 1 {
		t.Fatal("lock should be releasable after panic")
	}
}

// TestFire_cancelsHandlerOnMaxRunDuration 校验 MaxRunDuration 到期后 Handler 的 ctx 被 cancel
func TestFire_cancelsHandlerOnMaxRunDuration(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()
	factory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}

	canceled := make(chan struct{})
	fire(context.Background(), Job{
		Name:           "test-fire-max-run",
		MaxRunDuration: 150 * time.Millisecond,
	}, func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			close(canceled)
			return ctx.Err()
		case <-time.After(3 * time.Second):
			t.Error("handler ctx 未在 MaxRunDuration 到期后 cancel")
			return nil
		}
	}, factory)

	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("MaxRunDuration 到期后 Handler 未感知取消")
	}
}

// TestFire_zeroMaxRunDurationIgnoresFactoryDefault 校验 MaxRunDuration=0 不受工厂级 MaxHoldDuration 影响
func TestFire_zeroMaxRunDurationIgnoresFactoryDefault(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()
	factory, err := distlock.NewFactory(client,
		distlock.WithDefaultLockOptions(
			distlock.WithMaxHoldDuration(150*time.Millisecond),
			distlock.WithWatchdogInterval(50*time.Millisecond),
		),
	)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}

	canceled := make(chan struct{})
	fire(context.Background(), Job{Name: "test-fire-zero-max"}, func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			close(canceled)
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
			return nil
		}
	}, factory)

	select {
	case <-canceled:
		t.Fatal("MaxRunDuration=0 不应受工厂级 MaxHoldDuration 影响")
	default:
	}
}

// TestExecLockKey 校验锁 Key 前缀
func TestExecLockKey(t *testing.T) {
	if got := execLockKey("svc:job"); got != "cronjob:lock:svc:job" {
		t.Fatalf("got %s", got)
	}
}

// TestJobNameRegistry 校验同名 Job 拒绝注册与释放后可重新注册
func TestJobNameRegistry(t *testing.T) {
	name := "test-registry"
	if err := acquireJobName(name); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if err := acquireJobName(name); !errors.Is(err, ErrDuplicateJobName) {
		t.Fatalf("second acquire = %v, want ErrDuplicateJobName", err)
	}
	releaseJobName(name)
	if err := acquireJobName(name); err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	releaseJobName(name)
}

// TestSkipWarnThrottle 校验抢锁失败 Warn 的节流
func TestSkipWarnThrottle(t *testing.T) {
	name := "test-skip-warn"
	if err := acquireJobName(name); err != nil {
		t.Fatalf("acquireJobName: %v", err)
	}
	defer releaseJobName(name)

	if !shouldWarnSkip(name) {
		t.Fatal("first skip should warn")
	}
	if shouldWarnSkip(name) {
		t.Fatal("second skip within interval should not warn")
	}
}
