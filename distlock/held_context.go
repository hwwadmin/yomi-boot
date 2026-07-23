// Package distlock HeldContext 与业务 ctx 的合并绑定。
//
// @author zwei
package distlock

import "context"

// BindHeldContext 派生一个在 parent 取消或锁丢失时都会 Done 的 context。
// 未启用看门狗或未持锁时仅派生自 parent。
// 返回的 stop 必须调用以释放后台监听 goroutine；调用后可安全检查 HeldContext 状态。
func BindHeldContext(parent context.Context, lock Lock) (ctx context.Context, stop func()) {
	ctx, cancel := context.WithCancel(parent)
	if lock == nil {
		return ctx, cancel
	}
	held := lock.HeldContext()
	if held == nil {
		return ctx, cancel
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-held.Done():
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, func() {
		cancel()
		<-done
	}
}
