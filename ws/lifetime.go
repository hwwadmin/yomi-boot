// Package ws 实现服务端通道最大存活时间
//
// @author zwei
package ws

import (
	"math/rand/v2"
	"time"
)

const maxConnLifetimeCloseReason = "max lifetime exceeded"

// startLifetime 在握手成功后启动本连接的存活定时器；lifetime<=0 时不限制
func (c *connImpl) startLifetime(lifetime, jitter time.Duration) {
	if c == nil || lifetime <= 0 {
		return
	}
	actual := jitteredConnLifetime(lifetime, jitter)
	c.lifetimeTimer = time.AfterFunc(actual, func() {
		_ = c.Close(CloseGoingAway, maxConnLifetimeCloseReason)
	})
}

// stopLifetime 停止存活定时器；幂等
func (c *connImpl) stopLifetime() {
	if c == nil || c.lifetimeTimer == nil {
		return
	}
	c.lifetimeTimer.Stop()
	c.lifetimeTimer = nil
}

// jitteredConnLifetime 将基准寿命抖动到 [lifetime-jitter, lifetime+jitter]
func jitteredConnLifetime(lifetime, jitter time.Duration) time.Duration {
	if jitter <= 0 {
		return lifetime
	}
	min := lifetime - jitter
	max := lifetime + jitter
	return min + time.Duration(rand.Int64N(int64(max-min)+1))
}
