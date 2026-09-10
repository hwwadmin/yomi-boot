// Package wsbase 实现服务端通道最大存活时间：握手成功后按基准加减波动到期断开
//
// @author zwei
package wsbase

import (
	"math/rand/v2"
	"time"
)

const (
	maxConnLifetimeSessionKey  = "__yomi_ws_max_lifetime__"
	maxConnLifetimeCloseReason = "max lifetime exceeded"
)

// startMaxConnLifetime 在握手成功后启动本连接的存活定时器；lifetime<=0 时不限制
func startMaxConnLifetime(conn Conn, lifetime, jitter time.Duration) {
	if conn == nil || lifetime <= 0 {
		return
	}
	actual := jitteredConnLifetime(lifetime, jitter)
	timer := time.AfterFunc(actual, func() {
		_ = conn.Close(CloseGoingAway, maxConnLifetimeCloseReason)
	})
	if sess := conn.Session(); sess != nil {
		sess.Store(maxConnLifetimeSessionKey, timer)
	}
}

// stopMaxConnLifetime 停止存活定时器并清除 Session；幂等
func stopMaxConnLifetime(conn Conn) {
	if conn == nil {
		return
	}
	sess := conn.Session()
	if sess == nil {
		return
	}
	v, ok := sess.Load(maxConnLifetimeSessionKey)
	if !ok {
		return
	}
	sess.Delete(maxConnLifetimeSessionKey)
	if t, ok := v.(*time.Timer); ok && t != nil {
		t.Stop()
	}
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
