// Package wsbase 实现每连接心跳：周期 Ping 与读空闲超时
//
// @author zwei
package wsbase

import (
	"sync"
	"time"
)

const heartbeatSessionKey = "__yomi_ws_heartbeat__"

// heartbeatSettings 是已 Resolve 的心跳参数
type heartbeatSettings struct {
	interval time.Duration
	timeout  time.Duration
}

// enabled 是否需要启动 runner（发 Ping 或设读超时）
func (s heartbeatSettings) enabled() bool {
	return s.interval > 0 || s.timeout > 0
}

// heartbeatFromResolved 由 Resolve 结果构造 settings
func heartbeatFromResolved(interval, timeout time.Duration) heartbeatSettings {
	return heartbeatSettings{interval: interval, timeout: timeout}
}

// heartbeatRunner 管理单连接的 Ping 循环与 ReadDeadline 续命
type heartbeatRunner struct {
	conn     Conn
	interval time.Duration
	timeout  time.Duration
	stop     chan struct{}
	stopOnce sync.Once
}

// startHeartbeat 在连接上启动心跳；Timeout>0 时立即设 ReadDeadline
func startHeartbeat(conn Conn, s heartbeatSettings) {
	if conn == nil || !s.enabled() {
		return
	}
	r := &heartbeatRunner{
		conn:     conn,
		interval: s.interval,
		timeout:  s.timeout,
		stop:     make(chan struct{}),
	}
	if sess := conn.Session(); sess != nil {
		sess.Store(heartbeatSessionKey, r)
	}
	r.touch()
	if s.interval > 0 {
		go r.loop()
	}
}

// stopHeartbeat 停止心跳并清除 Session；幂等
func stopHeartbeat(conn Conn) {
	if conn == nil {
		return
	}
	sess := conn.Session()
	if sess == nil {
		return
	}
	v, ok := sess.Load(heartbeatSessionKey)
	if !ok {
		return
	}
	sess.Delete(heartbeatSessionKey)
	if r, ok := v.(*heartbeatRunner); ok && r != nil {
		r.Stop()
	}
}

// touchHeartbeat 用任意入站刷新读截止时间
func touchHeartbeat(conn Conn) {
	if conn == nil {
		return
	}
	sess := conn.Session()
	if sess == nil {
		return
	}
	v, ok := sess.Load(heartbeatSessionKey)
	if !ok {
		return
	}
	if r, ok := v.(*heartbeatRunner); ok && r != nil {
		r.touch()
	}
}

// touch 刷新 ReadDeadline；Timeout==0 时无操作
func (r *heartbeatRunner) touch() {
	if r == nil || r.conn == nil || r.timeout <= 0 {
		return
	}
	_ = r.conn.SetReadDeadline(time.Now().Add(r.timeout))
}

// Stop 停止 Ping 循环；幂等
func (r *heartbeatRunner) Stop() {
	if r == nil {
		return
	}
	r.stopOnce.Do(func() {
		close(r.stop)
	})
}

// loop 周期发送 Ping；写失败则关闭连接
func (r *heartbeatRunner) loop() {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-r.stop:
			return
		case <-ticker.C:
			if err := r.conn.WritePing(nil); err != nil {
				_ = r.conn.Close(CloseGoingAway, "heartbeat ping failed")
				return
			}
		}
	}
}
