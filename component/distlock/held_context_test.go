package distlock

import (
	"context"
	"testing"
	"time"
)

// stubHeldLock 仅用于 BindHeldContext 单测的最小 Lock 桩
type stubHeldLock struct {
	held context.Context
}

// Lock 实现 Lock 接口（测试桩）
func (s *stubHeldLock) Lock(context.Context) error { return nil }

// TryLock 实现 TryLock 接口（测试桩）
func (s *stubHeldLock) TryLock(context.Context) (bool, error) { return true, nil }

// TryLockWithTimeout 实现 TryLockWithTimeout 接口（测试桩）
func (s *stubHeldLock) TryLockWithTimeout(context.Context, time.Duration) (bool, error) {
	return true, nil
}

// Unlock 实现 Unlock 接口（测试桩）
func (s *stubHeldLock) Unlock(context.Context) error { return nil }

// HeldContext 实现 HeldContext 接口（测试桩）
func (s *stubHeldLock) HeldContext() context.Context { return s.held }

// Key 实现 Key 接口（测试桩）
func (s *stubHeldLock) Key() string { return "stub" }

// TestBindHeldContext_CancelsOnHeldDone 验证 HeldContext cancel 后派生 ctx 应 Done
func TestBindHeldContext_CancelsOnHeldDone(t *testing.T) {
	held, cancelHeld := context.WithCancel(context.Background())
	defer cancelHeld()

	ctx, stop := BindHeldContext(context.Background(), &stubHeldLock{held: held})
	defer stop()

	cancelHeld()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("HeldContext cancel 后派生 ctx 未 Done")
	}
}

// TestBindHeldContext_CancelsOnParentDone 验证 parent cancel 后派生 ctx 应 Done
func TestBindHeldContext_CancelsOnParentDone(t *testing.T) {
	held, cancelHeld := context.WithCancel(context.Background())
	defer cancelHeld()

	parent, cancelParent := context.WithCancel(context.Background())
	ctx, stop := BindHeldContext(parent, &stubHeldLock{held: held})
	defer stop()

	cancelParent()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("parent cancel 后派生 ctx 未 Done")
	}
}

// TestBindHeldContext_NilHeldStillTracksParent 验证 HeldContext 为 nil 时仍随 parent cancel
func TestBindHeldContext_NilHeldStillTracksParent(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	ctx, stop := BindHeldContext(parent, &stubHeldLock{held: nil})
	defer stop()

	cancelParent()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("无 HeldContext 时 parent cancel 后派生 ctx 未 Done")
	}
}

// TestBindHeldContext_NilLock 验证 lock 为 nil 时仍随 parent cancel
func TestBindHeldContext_NilLock(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	ctx, stop := BindHeldContext(parent, nil)
	defer stop()

	cancelParent()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("nil lock 时 parent cancel 后派生 ctx 未 Done")
	}
}

// TestBindHeldContext_StopReleasesGoroutine 验证 stop 应在合理时间内返回，避免 goroutine 泄漏
func TestBindHeldContext_StopReleasesGoroutine(t *testing.T) {
	held, cancelHeld := context.WithCancel(context.Background())
	defer cancelHeld()

	_, stop := BindHeldContext(context.Background(), &stubHeldLock{held: held})
	done := make(chan struct{})
	go func() {
		stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stop 未在 1s 内返回，监听 goroutine 可能泄漏")
	}
}
