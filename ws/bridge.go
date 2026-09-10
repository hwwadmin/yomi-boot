// Package ws 将 Config 解析出的 Handler 桥接为 wsbase.Handler
//
// @author zwei
package ws

import (
	"context"
	"sync"

	"gitee.com/hq_project/yomi-boot/ws/wsbase"

	"github.com/google/uuid"
)

// serverBridge 服务端桥接：自动生成 ConnID、维护 Hub、转发业务事件
type serverBridge struct {
	handler ServerHandler
	hub     *wsbase.Hub

	mu      sync.Mutex
	ids     map[wsbase.Conn]string // conn → connID，供 OnClose 使用
	closing bool                   // closing 为 true 时拒绝新登记，与 Register 同锁
}

// newServerBridge 创建服务端桥接
func newServerBridge(h ServerHandler, hub *wsbase.Hub) *serverBridge {
	if h == nil {
		h = NopServerHandler{}
	}
	return &serverBridge{
		handler: h,
		hub:     hub,
		ids:     make(map[wsbase.Conn]string),
	}
}

// beginClose 标记关闭中；与 OnOpen 登记同锁，避免 Close 后仍 Register
func (b *serverBridge) beginClose() {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.closing = true
	b.mu.Unlock()
}

// OnOpen 生成 ConnID、登记 Hub 并回调业务；关闭中则立即断开且不登记
func (b *serverBridge) OnOpen(conn wsbase.Conn) {
	connID := uuid.NewString()
	b.mu.Lock()
	if b.closing {
		b.mu.Unlock()
		_ = conn.Close(wsbase.CloseGoingAway, "server closing")
		return
	}
	if conn.Session() != nil {
		conn.Session().Store(sessionConnIDKey, connID)
	}
	b.hub.Register(connID, conn)
	b.ids[conn] = connID
	b.mu.Unlock()
	b.handler.OnConnect(&sessionContext{
		peerContext: peerContext{ctx: context.Background(), conn: conn},
		connID:      connID,
	})
}

// OnClose 注销 Hub 并回调业务
func (b *serverBridge) OnClose(conn wsbase.Conn, err error) {
	b.mu.Lock()
	connID := b.ids[conn]
	delete(b.ids, conn)
	b.mu.Unlock()
	if connID == "" {
		connID = loadConnID(conn)
	}
	if connID != "" {
		b.hub.Unregister(connID, conn)
	}
	b.handler.OnDisconnect(connID, err)
}

// OnPing 传输层已自动 Pong，业务不感知
func (b *serverBridge) OnPing(wsbase.Conn, []byte) {}

// OnPong 忽略
func (b *serverBridge) OnPong(wsbase.Conn, []byte) {}

// OnMessage 转发业务
func (b *serverBridge) OnMessage(conn wsbase.Conn, msg wsbase.Message) {
	b.mu.Lock()
	connID := b.ids[conn]
	b.mu.Unlock()
	if connID == "" {
		connID = loadConnID(conn)
	}
	b.handler.OnMessage(&sessionContext{
		peerContext: peerContext{ctx: context.Background(), conn: conn},
		connID:      connID,
	}, msg)
}

var _ wsbase.Handler = (*serverBridge)(nil)

// clientBridge 客户端桥接：单连接事件转发，无 ConnID
type clientBridge struct {
	handler  ClientHandler
	onDetach func(wsbase.Conn) // onDetach 在 OnClose 时通知门面清空连接引用

	mu   sync.Mutex
	conn wsbase.Conn
}

// newClientBridge 创建客户端桥接
func newClientBridge(h ClientHandler) *clientBridge {
	if h == nil {
		h = NopClientHandler{}
	}
	return &clientBridge{handler: h}
}

// setConn 更新当前连接引用
func (b *clientBridge) setConn(conn wsbase.Conn) {
	b.mu.Lock()
	b.conn = conn
	b.mu.Unlock()
}

// clearConn 清空当前连接引用
func (b *clientBridge) clearConn(conn wsbase.Conn) {
	b.mu.Lock()
	if b.conn == conn {
		b.conn = nil
	}
	b.mu.Unlock()
}

// OnOpen 回调业务 OnConnect
func (b *clientBridge) OnOpen(conn wsbase.Conn) {
	b.setConn(conn)
	b.handler.OnConnect(&peerContext{ctx: context.Background(), conn: conn})
}

// OnClose 回调业务 OnDisconnect，并通知门面断开当前连接引用
func (b *clientBridge) OnClose(conn wsbase.Conn, err error) {
	b.clearConn(conn)
	if b.onDetach != nil {
		b.onDetach(conn)
	}
	b.handler.OnDisconnect(err)
}

// OnPing 忽略
func (b *clientBridge) OnPing(wsbase.Conn, []byte) {}

// OnPong 忽略
func (b *clientBridge) OnPong(wsbase.Conn, []byte) {}

// OnMessage 转发业务
func (b *clientBridge) OnMessage(conn wsbase.Conn, msg wsbase.Message) {
	b.handler.OnMessage(&peerContext{ctx: context.Background(), conn: conn}, msg)
}

var _ wsbase.Handler = (*clientBridge)(nil)

// loadConnID 从 Session 读取框架写入的 ConnID
func loadConnID(conn wsbase.Conn) string {
	if conn == nil || conn.Session() == nil {
		return ""
	}
	v, ok := conn.Session().Load(sessionConnIDKey)
	if !ok {
		return ""
	}
	id, _ := v.(string)
	return id
}
