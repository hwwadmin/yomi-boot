// Package distlock RunWithLock 便捷函数：获取锁、执行业务函数、自动释放并合并锁丢失错误。
//
// @author zwei
package distlock

import (
	"context"
	"time"
	"yt-boot/appctx"
	"yt-boot/errors"
)

// RunWithLock 获取锁后执行 fn，结束时自动 Unlock。
// 启用看门狗时的推荐入口：fn 收到 HeldContext（锁丢失时 cancel），
// 正常返回 nil 但锁已丢失时返回 ErrLockLost。
// 未启用看门狗时也可使用，行为等价于 Lock + defer Unlock。
func RunWithLock(ctx context.Context, lock Lock, fn func(ctx context.Context) error) (err error) {
	if lock == nil {
		return errors.New("distlock: lock 为 nil")
	}
	if fn == nil {
		return errors.New("distlock: fn 为 nil")
	}
	ctx = appctx.OrBackground(ctx)

	if err = lock.Lock(ctx); err != nil {
		return err
	}
	defer func() {
		// 使用带超时的 context 防止 Unlock 在 Redis 不可用时永久阻塞
		unlockCtx, cancel := appctx.BackgroundWithTimeout(5 * time.Second)
		defer cancel()
		unlockErr := lock.Unlock(unlockCtx)
		// fn 返回错误时也应告知调用方互斥保护已失效
		if errors.Is(unlockErr, ErrLockLost) {
			err = errors.Join(ErrLockLost, err)
		}
	}()

	// runCtx 派生自用户 ctx，同时监听 HeldContext：任一取消都让 fn 收到取消信号。
	runCtx, stop := BindHeldContext(ctx, lock)
	err = fn(runCtx)
	// stop 先 cancel 再等待监听 goroutine，消除「fn 返回 → held.Err() 检查」的 TOCTOU 窗口。
	stop()

	if err != nil {
		// fn 执行期间锁可能丢失，需要显式告知调用方互斥保护已失效
		if held := lock.HeldContext(); held != nil && held.Err() != nil {
			return errors.Join(ErrLockLost, err)
		}
		return err
	}
	if held := lock.HeldContext(); held != nil && held.Err() != nil {
		return errors.Sentinel(ErrLockLost)
	}
	return nil
}
