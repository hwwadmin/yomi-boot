// Package wsapp 将 Config 解析出的 Handler 桥接为 ws.Handler
//
// @author zwei
package wsapp

import (
	"context"
	"sync"

	"gitee.com/hq_project/yomi-boot/ws"

	"github.com/google/uuid"
)

// serverBridge 服务端桥接：自动生成 ConnID、维护 Hub、转发业务事件
type serverBridge struct {
	handler ServerHandler
	hub     *ws.Hub

	mu  sync.Mutex
	ids map[ws.Conn]string // conn → connID，供 OnClose 使用
}

// newServerBridge 创建服务端桥接
func newServerBridge(h ServerHandler, hub *ws.Hub) *serverBridge {
	if h == nil {
		h = NopServerHandler{}
	}
	return &serverBridge{
		handler: h,
		hub:     hub,
		ids:     make(map[ws.Conn]string),
	}
}

// OnOpen 生成 ConnID、登记 Hub 并回调业务
func (b *serverBridge) OnOpen(conn ws.Conn) {
	connID := uuid.NewString()
	if conn.Session() != nil {
		conn.Session().Store(sessionConnIDKey, connID)
	}
	b.hub.Register(connID, conn)
	b.mu.Lock()
	b.ids[conn] = connID
	b.mu.Unlock()
	b.handler.OnConnect(&sessionContext{
		peerContext: peerContext{ctx: context.Background(), conn: conn},
		connID:      connID,
	})
}

// OnClose 注销 Hub 并回调业务
func (b *serverBridge) OnClose(conn ws.Conn, err error) {
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
func (b *serverBridge) OnPing(ws.Conn, []byte) {}

// OnPong 忽略
func (b *serverBridge) OnPong(ws.Conn, []byte) {}

// OnMessage 转发业务
func (b *serverBridge) OnMessage(conn ws.Conn, msg ws.Message) {
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

var _ ws.Handler = (*serverBridge)(nil)

// clientBridge 客户端桥接：单连接事件转发，无 ConnID
type clientBridge struct {
	handler ClientHandler

	mu   sync.Mutex
	conn ws.Conn
}

// newClientBridge 创建客户端桥接
func newClientBridge(h ClientHandler) *clientBridge {
	if h == nil {
		h = NopClientHandler{}
	}
	return &clientBridge{handler: h}
}

// setConn 更新当前连接引用
func (b *clientBridge) setConn(conn ws.Conn) {
	b.mu.Lock()
	b.conn = conn
	b.mu.Unlock()
}

// clearConn 清空当前连接引用
func (b *clientBridge) clearConn(conn ws.Conn) {
	b.mu.Lock()
	if b.conn == conn {
		b.conn = nil
	}
	b.mu.Unlock()
}

// OnOpen 回调业务 OnConnect
func (b *clientBridge) OnOpen(conn ws.Conn) {
	b.setConn(conn)
	b.handler.OnConnect(&peerContext{ctx: context.Background(), conn: conn})
}

// OnClose 回调业务 OnDisconnect
func (b *clientBridge) OnClose(conn ws.Conn, err error) {
	b.clearConn(conn)
	b.handler.OnDisconnect(err)
}

// OnPing 忽略
func (b *clientBridge) OnPing(ws.Conn, []byte) {}

// OnPong 忽略
func (b *clientBridge) OnPong(ws.Conn, []byte) {}

// OnMessage 转发业务
func (b *clientBridge) OnMessage(conn ws.Conn, msg ws.Message) {
	b.handler.OnMessage(&peerContext{ctx: context.Background(), conn: conn}, msg)
}

var _ ws.Handler = (*clientBridge)(nil)

// loadConnID 从 Session 读取框架写入的 ConnID
func loadConnID(conn ws.Conn) string {
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
