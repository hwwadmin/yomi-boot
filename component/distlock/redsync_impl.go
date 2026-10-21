// Package distlock 基于 go-redsync 的 Lock 与 DistLockFactory 实现。
//
// @author zwei
package distlock

import (
	"context"
	"sync"
	"time"

	"gitee.com/hq_project/yomi-boot/boot/appctx"
	"gitee.com/hq_project/yomi-boot/boot/errors"

	"github.com/go-redsync/redsync/v4"
	goredis "github.com/go-redsync/redsync/v4/redis/goredis/v9"
	redisv9 "github.com/redis/go-redis/v9"
)

var (
	_ DistLockFactory = (*redsyncFactory)(nil)
	_ Lock            = (*redsyncLock)(nil)
)

// redsyncFactory 是基于 redsync 的 DistLockFactory 实现
type redsyncFactory struct {
	rs      *redsync.Redsync // rs 是 redsync 实例
	options ServiceOptions   // options 是工厂级配置
}

// redsyncLock 是基于 redsync 的 Lock 实现
type redsyncLock struct {
	factory *redsyncFactory // factory 是所属工厂
	key     string          // key 是完整锁 Key（含前缀）
	opts    LockOptions     // opts 是加锁参数

	mu    sync.Mutex     // mu 保护本实例持锁状态
	mutex *redsync.Mutex // mutex 是 redsync 互斥量，nil 表示未持锁

	watchdogStop  chan struct{}      // watchdogStop 关闭后通知看门狗退出
	watchdogDone  chan struct{}      // watchdogDone 在看门狗退出后关闭
	heldCtx       context.Context    // heldCtx 在锁丢失时 cancel
	heldCancel    context.CancelFunc // heldCancel 取消 heldCtx
	holdStartedAt time.Time          // holdStartedAt 记录持锁起始时间
	lost          bool               // lost 表示续期失败或达到 MaxHoldDuration
}

// newRedsyncFactory 创建 redsync 工厂实现
func newRedsyncFactory(client *redisv9.Client, opts ...ServiceOption) (DistLockFactory, error) {
	if client == nil {
		return nil, errors.New("distlock: redis 客户端未初始化: client 为 nil")
	}
	pool := goredis.NewPool(client)
	return &redsyncFactory{
		rs:      redsync.New(pool),
		options: applyServiceOptions(opts),
	}, nil
}

// GetLock 创建绑定指定 key 的 Lock 实例
func (f *redsyncFactory) GetLock(key string, opts ...LockOption) Lock {
	lockOpts := applyLockOptions(f.options.Default, opts)
	return &redsyncLock{
		factory: f,
		key:     key,
		opts:    lockOpts,
	}
}

// Key 返回绑定的锁 Key
func (l *redsyncLock) Key() string {
	return l.key
}

// Lock 阻塞获取锁直到成功，或 ctx 取消/超时时返回错误
func (l *redsyncLock) Lock(ctx context.Context) error {
	ctx = appctx.OrBackground(ctx)
	return l.lockContextWithMutex(ctx, l.opts.AcquireTries)
}

// TryLock 非阻塞尝试获取锁
func (l *redsyncLock) TryLock(ctx context.Context) (bool, error) {
	ctx = appctx.OrBackground(ctx)

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.mutex != nil {
		return false, l.holdConflictErrLocked()
	}

	mutex := l.newMutex(1)
	err := mutex.TryLockContext(ctx)
	if err == nil {
		l.mutex = mutex
		l.onHoldAcquiredLocked()
		return true, nil
	}
	if mappedErr := mapLockContextError(ctx, err, false); mappedErr != nil {
		return false, mappedErr
	}
	return false, nil
}

// TryLockWithTimeout 在 timeout 内尝试获取锁
func (l *redsyncLock) TryLockWithTimeout(ctx context.Context, timeout time.Duration) (bool, error) {
	if timeout <= 0 {
		return l.TryLock(ctx)
	}
	ctx = appctx.OrBackground(ctx)

	timeoutCtx, cancel := appctx.WithTimeout(ctx, timeout)
	defer cancel()

	err := l.lockContextWithMutex(timeoutCtx, 1)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, nil
	}
	if errors.Is(err, context.Canceled) {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, err
	}
	return false, errors.Wrap(err, "尝试获取分布式锁失败")
}

// Unlock 释放当前实例持有的锁。
//
// 顺序说明：先停止看门狗再 UnlockContext，防止 Unlock 成功后看门狗又 Extend 把锁加回。
// 代价是 UnlockContext 失败时看门狗已停，重试期间不再续期，锁将随 TTL 自然过期（可接受的降级）。
// 若看门狗已标记 lost，仍会尝试 UnlockContext 清理 Redis 残留（用持锁 token 匹配，不会误删他人锁），
// 但返回 ErrLockLost 提醒调用方锁已丢失。
// 若 Redis 侧锁已失效（Key 过期/被删，或 Key 已被其他实例持有），同样清理本地持锁状态并
// 返回 ErrLockLost；ErrLockNotHeld 仅表示实例从未持有或已释放。
func (l *redsyncLock) Unlock(ctx context.Context) error {
	ctx = appctx.OrBackground(ctx)

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.mutex == nil {
		return errors.Sentinel(ErrLockNotHeld)
	}

	lost := l.lost
	l.stopWatchdogLocked()
	// stopWatchdogLocked 在 <-watchdogDone 期间会释放并重新获取 l.mu，
	// 此期间看门狗可能刚执行 markLostLocked() 将 l.lost 置 true，
	// 需要重新读取以避免漏掉此状态误返回 nil。
	lost = lost || l.lost
	mutex := l.mutex
	ok, err := mutex.UnlockContext(ctx)
	if err != nil {
		// Key 已不存在（过期/被删）或已被其他实例持有：本地锁已失效，
		// 必须清理状态，否则实例会永久停留在"持锁"状态且无法再次使用。
		var taken *redsync.ErrTaken
		if errors.Is(err, redsync.ErrLockAlreadyExpired) || errors.As(err, &taken) {
			l.mutex = nil
			l.lost = false
			return errors.Sentinel(ErrLockLost)
		}
		if lost {
			return errors.Sentinel(ErrLockLost)
		}
		return errors.Wrap(err, "释放分布式锁失败")
	}
	l.mutex = nil
	l.lost = false
	if lost {
		return errors.Sentinel(ErrLockLost)
	}
	if !ok {
		// 当前单 pool 工厂下 UnlockContext 不会返回 (false, nil)；保留为防御分支。
		return errors.Sentinel(ErrLockLost)
	}
	return nil
}

// lockContextWithMutex 在 ctx 有效期内循环尝试获取锁
func (l *redsyncLock) lockContextWithMutex(ctx context.Context, triesPerAttempt int) error {
	if triesPerAttempt <= 0 {
		triesPerAttempt = 1
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.mutex != nil {
		return l.holdConflictErrLocked()
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		mutex := l.newMutex(triesPerAttempt)
		if err := mutex.LockContext(ctx); err == nil {
			l.mutex = mutex
			l.onHoldAcquiredLocked()
			return nil
		} else if err := mapLockContextError(ctx, err, true); err != nil {
			return err
		}

		if !waitRetry(ctx, l.opts.AcquireRetryDelay) {
			return ctx.Err()
		}
	}
}

// holdConflictErrLocked 在实例已持锁时返回冲突错误；调用方须已持有 l.mu
func (l *redsyncLock) holdConflictErrLocked() error {
	if l.lost {
		return errors.Sentinel(ErrLockLost)
	}
	return errors.Sentinel(ErrLockAlreadyHeld)
}

// newMutex 按当前锁参数创建 redsync Mutex
func (l *redsyncLock) newMutex(tries int) *redsync.Mutex {
	opts := []redsync.Option{
		redsync.WithExpiry(l.opts.TTL),
		redsync.WithTries(tries),
	}
	opts = append(opts, redsync.WithRetryDelay(l.opts.AcquireRetryDelay))
	return l.factory.rs.NewMutex(l.key, opts...)
}

// mapLockContextError 将 redsync 加锁错误映射为可重试或向上传递的错误。
// retryRedisErr 为 true 时 Redis 通信错误返回 nil，供 Lock / TryLockWithTimeout 外层重试；
// TryLock 应传 false，以便调用方区分基础设施错误与锁被占用。
func mapLockContextError(ctx context.Context, err error, retryRedisErr bool) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if isAcquireFailure(err) || (retryRedisErr && isRetryableError(err)) {
		return nil
	}
	return errors.Wrap(err, "获取分布式锁失败")
}

// isRetryableError 判断是否为可重试错误（Redis 通信错误或 context 超时/取消）。
// context.DeadlineExceeded / context.Canceled 表示操作超时或取消，不代表锁确已丢失，
// 应允许外层通过 consecutiveFailures 机制重试，而非立即标记锁丢失。
func isRetryableError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var redisErr *redsync.RedisError
	return errors.As(err, &redisErr)
}

// isAcquireFailure 判断是否为锁已被占用导致的获取失败
func isAcquireFailure(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, redsync.ErrFailed) {
		return true
	}
	var taken *redsync.ErrTaken
	return errors.As(err, &taken)
}

// waitRetry 等待下一次获取尝试，ctx 取消时返回 false
func waitRetry(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
