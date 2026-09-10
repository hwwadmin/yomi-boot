// Package ws 将业务 Handler 适配为 gws.Event
//
// @author zwei
package ws

import "github.com/lxzan/gws"

// eventAdapter 实现 gws.Event，把 *gws.Conn 转为门面 Conn 再回调业务
type eventAdapter struct {
	handler   Handler
	chunkSize int
}

// newEventAdapter 创建事件适配器；h 为 nil 时使用 NopHandler
func newEventAdapter(h Handler, chunkSize int) *eventAdapter {
	if h == nil {
		h = NopHandler{}
	}
	return &eventAdapter{handler: h, chunkSize: resolveChunkSize(chunkSize)}
}

// OnOpen 建立连接
func (a *eventAdapter) OnOpen(socket *gws.Conn) {
	conn := connFromGWS(socket, a.chunkSize)
	a.handler.OnOpen(conn)
}

// OnClose 连接关闭
func (a *eventAdapter) OnClose(socket *gws.Conn, err error) {
	conn := connFromGWS(socket, a.chunkSize)
	a.handler.OnClose(conn, err)
}

// OnPing 先自动 Pong，再回调业务
func (a *eventAdapter) OnPing(socket *gws.Conn, payload []byte) {
	_ = socket.WritePong(payload)
	conn := connFromGWS(socket, a.chunkSize)
	a.handler.OnPing(conn, payload)
}

// OnPong 收到 Pong
func (a *eventAdapter) OnPong(socket *gws.Conn, payload []byte) {
	conn := connFromGWS(socket, a.chunkSize)
	a.handler.OnPong(conn, payload)
}

// OnMessage 收到完整消息
func (a *eventAdapter) OnMessage(socket *gws.Conn, message *gws.Message) {
	conn := connFromGWS(socket, a.chunkSize)
	a.handler.OnMessage(conn, &messageView{raw: message})
}

var _ gws.Event = (*eventAdapter)(nil)
