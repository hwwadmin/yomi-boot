// Package ws 实现每连接心跳：周期 Ping 与读空闲超时
//
// @author zwei
package ws

import (
	"sync"
	"time"
)

// heartbeatSettings 是已 Resolve 的心跳参数
type heartbeatSettings struct {
	interval time.Duration
	timeout  time.Duration
}

// enabled 是否需要启动 runner（发 Ping 或设空闲超时）
func (s heartbeatSettings) enabled() bool {
	return s.interval > 0 || s.timeout > 0
}

// heartbeatFromResolved 由 Resolve 结果构造 settings
func heartbeatFromResolved(interval, timeout time.Duration) heartbeatSettings {
	return heartbeatSettings{interval: interval, timeout: timeout}
}

// heartbeatRunner 管理单连接的 Ping 循环与空闲超时
type heartbeatRunner struct {
	conn     *connImpl
	interval time.Duration
	timeout  time.Duration
	stop     chan struct{}
	stopOnce sync.Once

	mu    sync.Mutex
	timer *time.Timer
}

// startHeartbeat 在连接上启动心跳
func (c *connImpl) startHeartbeat(s heartbeatSettings) {
	if c == nil || !s.enabled() {
		return
	}
	r := &heartbeatRunner{
		conn:     c,
		interval: s.interval,
		timeout:  s.timeout,
		stop:     make(chan struct{}),
	}
	c.heartbeat = r
	r.touch()
	if s.interval > 0 {
		go r.loop()
	}
}

// stopHeartbeat 停止心跳；幂等
func (c *connImpl) stopHeartbeat() {
	if c == nil || c.heartbeat == nil {
		return
	}
	c.heartbeat.Stop()
}

// touchHeartbeat 用任意入站刷新空闲超时
func (c *connImpl) touchHeartbeat() {
	if c == nil || c.heartbeat == nil {
		return
	}
	c.heartbeat.touch()
}

// touch 刷新空闲定时器；Timeout==0 或已停止时无操作
func (r *heartbeatRunner) touch() {
	if r == nil || r.timeout <= 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	select {
	case <-r.stop:
		return
	default:
	}
	if r.timer == nil {
		r.timer = time.AfterFunc(r.timeout, r.onIdle)
		return
	}
	r.timer.Reset(r.timeout)
}

// onIdle 空闲超时后主动断开
func (r *heartbeatRunner) onIdle() {
	if r == nil || r.conn == nil {
		return
	}
	select {
	case <-r.stop:
		return
	default:
	}
	_ = r.conn.Close(CloseGoingAway, "heartbeat timeout")
}

// Stop 停止 Ping 循环与空闲定时器；幂等
func (r *heartbeatRunner) Stop() {
	if r == nil {
		return
	}
	r.stopOnce.Do(func() {
		close(r.stop)
	})
	r.mu.Lock()
	if r.timer != nil {
		r.timer.Stop()
	}
	r.mu.Unlock()
}

// loop 周期经写队列插队发送 Ping；失败则关闭连接
func (r *heartbeatRunner) loop() {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-r.stop:
			return
		case <-ticker.C:
			wait := r.timeout
			if wait <= 0 {
				wait = r.interval
			}
			err := r.conn.enqueuePing(wait)
			if err != nil {
				_ = r.conn.Close(CloseGoingAway, "heartbeat ping failed")
				return
			}
			r.touch()
		}
	}
}
