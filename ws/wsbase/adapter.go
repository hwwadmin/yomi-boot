// Package wsbase 将业务 Handler 适配为 gws.Event
//
// @author zwei
package wsbase

import "github.com/lxzan/gws"

// eventAdapter 实现 gws.Event，把 *gws.Conn 转为门面 Conn 再回调业务
type eventAdapter struct {
	handler   Handler
	heartbeat heartbeatSettings
}

// newEventAdapter 创建事件适配器；h 为 nil 时使用 NopHandler
func newEventAdapter(h Handler, hb heartbeatSettings) *eventAdapter {
	if h == nil {
		h = NopHandler{}
	}
	return &eventAdapter{handler: h, heartbeat: hb}
}

// OnOpen 建立连接并启动心跳
func (a *eventAdapter) OnOpen(socket *gws.Conn) {
	conn := connFromGWS(socket)
	a.handler.OnOpen(conn)
	startHeartbeat(conn, a.heartbeat)
}

// OnClose 停止存活定时器与心跳并回调业务
func (a *eventAdapter) OnClose(socket *gws.Conn, err error) {
	conn := connFromGWS(socket)
	stopMaxConnLifetime(conn)
	stopHeartbeat(conn)
	a.handler.OnClose(conn, err)
}

// OnPing 先自动 Pong，续命后再回调业务
func (a *eventAdapter) OnPing(socket *gws.Conn, payload []byte) {
	_ = socket.WritePong(payload)
	conn := connFromGWS(socket)
	touchHeartbeat(conn)
	a.handler.OnPing(conn, payload)
}

// OnPong 续命后回调业务
func (a *eventAdapter) OnPong(socket *gws.Conn, payload []byte) {
	conn := connFromGWS(socket)
	touchHeartbeat(conn)
	a.handler.OnPong(conn, payload)
}

// OnMessage 续命后回调业务（先续命，避免慢 Processor 拖死连接）
func (a *eventAdapter) OnMessage(socket *gws.Conn, message *gws.Message) {
	conn := connFromGWS(socket)
	touchHeartbeat(conn)
	a.handler.OnMessage(conn, &messageView{raw: message})
}

var _ gws.Event = (*eventAdapter)(nil)
