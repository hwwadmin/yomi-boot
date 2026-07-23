// Package distlock 定义分布式锁相关的 sentinel 错误。
//
// @author zwei
package distlock

import "yt-boot/errors"

var (
	// ErrLockNotHeld 当前实例未持有锁时调用 Unlock
	ErrLockNotHeld = errors.NewNoStack("distlock: 当前未持有锁")

	// ErrLockAlreadyHeld 当前实例已持有锁时再次加锁
	ErrLockAlreadyHeld = errors.NewNoStack("distlock: 当前已持有锁")

	// ErrLockLost 看门狗续期失败或锁在 Redis 侧已失效，不可再假设互斥成立
	ErrLockLost = errors.NewNoStack("distlock: 锁已丢失")
)
