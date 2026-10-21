// Package distlock Lock 接口定义，声明分布式互斥锁的核心方法。
//
// @author zwei
package distlock

import (
	"context"
	"time"
)

// Lock 分布式互斥锁，每个实例绑定一个 Redis Key。
// 同一实例应由单 goroutine 串行调用，并发模型详见包文档。
type Lock interface {
	// Lock 阻塞获取锁直到成功，或 ctx 取消/超时时返回错误
	Lock(ctx context.Context) error

	// TryLock 非阻塞尝试获取锁，成功返回 (true, nil)；锁被占用返回 (false, nil)；
	// 基础设施错误返回 (false, err)，不重试。
	TryLock(ctx context.Context) (bool, error)

	// TryLockWithTimeout 在 timeout 内尝试获取锁。
	// 自身超时返回 (false, nil)；ctx 取消/超时返回 (false, err)。
	TryLockWithTimeout(ctx context.Context, timeout time.Duration) (bool, error)

	// Unlock 释放当前实例持有的锁
	Unlock(ctx context.Context) error

	// HeldContext 返回一个 context，锁丢失时会被 cancel。
	// 未启用看门狗或未持锁时返回 nil。
	HeldContext() context.Context

	// Key 返回绑定的锁 Key
	Key() string
}
