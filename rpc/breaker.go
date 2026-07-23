// Package rpc 提供按下游服务粒度的轻量熔断器
//
// @author zwei
package rpc

import (
	"sync"
	"time"
	"yt-boot/errors"
)

// breakerState 熔断器状态
type breakerState int

const (
	breakerClosed breakerState = iota
	breakerOpen
	breakerHalfOpen
)

// circuitBreaker 三态熔断：Closed → Open → HalfOpen → Closed
type circuitBreaker struct {
	mu sync.Mutex

	failures      int
	threshold     int
	openDuration  time.Duration
	state         breakerState
	openUntil     time.Time
	probeInFlight bool
}

// newCircuitBreaker 创建熔断器；非法参数回落到默认阈值与打开时长
func newCircuitBreaker(threshold int, openDuration time.Duration) *circuitBreaker {
	if threshold <= 0 {
		threshold = defaultBreakerFailures
	}
	if openDuration <= 0 {
		openDuration = defaultBreakerOpenDuration
	}
	return &circuitBreaker{
		threshold:    threshold,
		openDuration: openDuration,
		state:        breakerClosed,
	}
}

// allow 判断当前是否允许发起调用；熔断打开时返回 ErrCircuitOpen
func (b *circuitBreaker) allow() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	switch b.state {
	case breakerOpen:
		if now.Before(b.openUntil) {
			return errors.Sentinel(ErrCircuitOpen)
		}
		b.state = breakerHalfOpen
		b.probeInFlight = true
		return nil
	case breakerHalfOpen:
		if b.probeInFlight {
			return errors.Sentinel(ErrCircuitOpen)
		}
		b.probeInFlight = true
		return nil
	default:
		return nil
	}
}

// onSuccess 传输成功（含拿到业务信封/业务错误），重置熔断
func (b *circuitBreaker) onSuccess() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = 0
	b.probeInFlight = false
	b.state = breakerClosed
}

// onFailure 传输类失败，累计后可能打开熔断
func (b *circuitBreaker) onFailure() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.probeInFlight = false
	if b.state == breakerHalfOpen {
		b.state = breakerOpen
		b.openUntil = time.Now().Add(b.openDuration)
		b.failures = b.threshold
		return
	}
	b.failures++
	if b.failures >= b.threshold {
		b.state = breakerOpen
		b.openUntil = time.Now().Add(b.openDuration)
	}
}
