// Package distlock RunWithLock 便捷函数：获取锁、执行业务函数、自动释放并合并锁丢失错误。
//
// @author zwei
package distlock

import (
	"context"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/appctx"
	"gitee.com/hq_project/yomi-boot/core/errors"
)

// RunWithLock 获取锁后执行 fn，结束时自动 Unlock。
// 启用看门狗时的推荐入口：fn 收到 HeldContext（锁丢失时 cancel），
// 正常返回 nil 但锁已丢失时返回 ErrLockLost。
// 释放锁失败（非 ErrLockNotHeld）时，返回错误会同时保留 fn 的错误与释放失败原因。
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
		switch {
		case unlockErr == nil:
		case errors.Is(unlockErr, ErrLockLost):
			// fn 返回错误时也应告知调用方互斥保护已失效
			err = errors.Join(ErrLockLost, err)
		case errors.Is(unlockErr, ErrLockNotHeld):
			// RunWithLock 成功获取过锁；此分支只可能是 fn 内自行 Unlock（或实例已被释放），
			// 不属于锁丢失，无需上报。
		default:
			// 释放失败同样不能被吞掉，否则调用方会误以为锁已释放
			err = errors.Join(errors.Wrap(unlockErr, "释放分布式锁失败"), err)
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
