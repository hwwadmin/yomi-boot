// Package distlock 看门狗续期逻辑：周期性续期、连续失败检测、锁丢失标记。
//
// @author zwei
package distlock

import (
	"context"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/appctx"
	"gitee.com/hq_project/yomi-boot/component/core/logger"
)

// watchdogInterval 返回看门狗续期间隔，默认 TTL/3
func (l *redsyncLock) watchdogInterval() time.Duration {
	if l.opts.WatchdogInterval > 0 {
		return l.opts.WatchdogInterval
	}
	interval := l.opts.TTL / 3
	if interval <= 0 {
		interval = time.Second
	}
	return interval
}

// onHoldAcquiredLocked 在成功持锁后启动看门狗；调用方须已持有 l.mu
func (l *redsyncLock) onHoldAcquiredLocked() {
	l.lost = false
	l.holdStartedAt = time.Now()
	if !l.opts.Watchdog {
		return
	}

	l.heldCtx, l.heldCancel = appctx.BackgroundWithCancel()
	l.watchdogStop = make(chan struct{})
	l.watchdogDone = make(chan struct{})
	go l.runWatchdog()
}

// stopWatchdogLocked 通知看门狗退出并等待其结束，然后清理 HeldContext 相关字段。
// 调用方须已持有 l.mu，但本方法在 <-watchdogDone 期间会释放并重新获取 l.mu，
// 让看门狗 markLost 路径能完成（其需要 l.mu），避免与本方法互相等待死锁。
func (l *redsyncLock) stopWatchdogLocked() {
	if l.watchdogStop == nil {
		// 仍需清理 HeldContext（未启用看门狗时也会走到这里）
		if l.heldCancel != nil {
			l.heldCancel()
			l.heldCancel = nil
		}
		l.heldCtx = nil
		return
	}
	// 防御性关闭：如果 l.mu 释放期间有其他调用方已关闭 watchdogStop，
	// 直接关闭已关闭的 channel 会 panic，改用 select 安全关闭。
	select {
	case <-l.watchdogStop:
		// channel 已关闭
	default:
		close(l.watchdogStop)
	}
	done := l.watchdogDone
	l.mu.Unlock()
	<-done
	l.mu.Lock()
	l.watchdogStop = nil
	l.watchdogDone = nil

	if l.heldCancel != nil {
		l.heldCancel()
		l.heldCancel = nil
	}
	l.heldCtx = nil
}

// markLostLocked 标记锁已丢失并 cancel HeldContext；调用方须已持有 l.mu
func (l *redsyncLock) markLostLocked() {
	if l.lost {
		return
	}
	l.lost = true
	if l.heldCancel != nil {
		l.heldCancel()
	}
}

// runWatchdog 周期性续期，连续失败或达到 MaxHoldDuration 时标记丢失
func (l *redsyncLock) runWatchdog() {
	defer close(l.watchdogDone)
	// panic 兜底：续期已无法继续，必须标记锁丢失并 cancel HeldContext，
	// 避免业务在无感知下持有一把注定过期的锁（done 仍会关闭，Unlock 不会卡住）
	defer func() {
		if r := recover(); r != nil {
			logger.ErrorWithoutCtx("distlock 看门狗 goroutine panic，标记锁丢失",
				"key", l.key, "panic", r)
			l.mu.Lock()
			if l.mutex != nil {
				l.markLostLocked()
			}
			l.mu.Unlock()
		}
	}()

	consecutiveFailures := 0
	if stop, next := l.tryWatchdogExtend(consecutiveFailures); stop {
		return
	} else {
		consecutiveFailures = next
	}

	interval := l.watchdogInterval()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-l.watchdogStop:
			return
		case <-ticker.C:
			stop, next := l.tryWatchdogExtend(consecutiveFailures)
			consecutiveFailures = next
			if stop {
				return
			}
		}
	}
}

// tryWatchdogExtend 尝试一次续期；stop=true 表示看门狗应退出
func (l *redsyncLock) tryWatchdogExtend(consecutiveFailures int) (stop bool, nextFailures int) {
	if l.shouldStopWatchdogTick() {
		return true, consecutiveFailures
	}

	extendCtx, cancel := extendContext(l.opts.TTL)
	ok, err := l.extendHeldMutex(extendCtx)
	cancel()

	if ok && err == nil {
		return false, 0
	}

	consecutiveFailures++
	if consecutiveFailures >= defaultWatchdogExtendFailures || !isRetryableError(err) {
		l.mu.Lock()
		if l.mutex != nil {
			l.markLostLocked()
		}
		l.mu.Unlock()
		return true, consecutiveFailures
	}
	return false, consecutiveFailures
}

// shouldStopWatchdogTick 检查是否应停止看门狗；持锁释放或达到 MaxHoldDuration 时返回 true
func (l *redsyncLock) shouldStopWatchdogTick() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.mutex == nil || l.lost {
		return true
	}
	if l.opts.MaxHoldDuration > 0 && time.Since(l.holdStartedAt) >= l.opts.MaxHoldDuration {
		l.markLostLocked()
		return true
	}
	return false
}

// extendHeldMutex 在持锁状态下执行一次续期；调用方不应持有 l.mu
func (l *redsyncLock) extendHeldMutex(ctx context.Context) (bool, error) {
	l.mu.Lock()
	mutex := l.mutex
	l.mu.Unlock()

	if mutex == nil {
		return false, ErrLockNotHeld
	}
	return mutex.ExtendContext(ctx)
}

// extendContext 为续期操作创建独立超时 context，不绑定业务 ctx。
// 超时取 TTL 的 5% 与 100ms 的较大值，且不超过 TTL/2，避免续期进行中锁已过期。
func extendContext(ttl time.Duration) (context.Context, context.CancelFunc) {
	timeout := ttl / 20
	if timeout < 100*time.Millisecond {
		timeout = 100 * time.Millisecond
	}
	if cap := ttl / 2; cap > 0 && timeout > cap {
		timeout = cap
	}
	return appctx.BackgroundWithTimeout(timeout)
}

// HeldContext 返回锁丢失时会被 cancel 的 context
func (l *redsyncLock) HeldContext() context.Context {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.heldCtx
}

// isLost 返回当前实例是否已丢失锁（看门狗续期失败或达到 MaxHoldDuration）
func (l *redsyncLock) isLost() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.lost
}
