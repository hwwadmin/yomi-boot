// Package ws 定义事件 Handler 与会话存储
//
// @author zwei
package ws

import "github.com/lxzan/gws"

// Handler 是连接生命周期与消息回调；业务实现此接口，勿直接实现 gws.Event
type Handler interface {
	// OnOpen 连接建立
	OnOpen(conn Conn)
	// OnClose 连接关闭；若对端发送关闭帧，err 可能为 *gws.CloseError
	OnClose(conn Conn, err error)
	// OnPing 收到 Ping；库会先自动回复 Pong，再回调本方法
	OnPing(conn Conn, payload []byte)
	// OnPong 收到 Pong
	OnPong(conn Conn, payload []byte)
	// OnMessage 收到完整文本/二进制消息；用毕须 msg.Close()
	OnMessage(conn Conn, msg Message)
}

// NopHandler 空实现，可作为嵌入基类
type NopHandler struct{}

// OnOpen 空实现
func (NopHandler) OnOpen(Conn) {}

// OnClose 空实现
func (NopHandler) OnClose(Conn, error) {}

// OnPing 空实现
func (NopHandler) OnPing(Conn, []byte) {}

// OnPong 空实现
func (NopHandler) OnPong(Conn, []byte) {}

// OnMessage 关闭消息缓冲后返回
func (NopHandler) OnMessage(_ Conn, msg Message) {
	if msg != nil {
		_ = msg.Close()
	}
}

// Session 是连接级键值存储
type Session interface {
	// Load 按键取值
	Load(key string) (value any, ok bool)
	// Store 存键值
	Store(key string, value any)
	// Delete 删键
	Delete(key string)
	// Len 返回条目数
	Len() int
}

// sessionAdapter 将 gws.SessionStorage 适配为 Session
type sessionAdapter struct {
	raw gws.SessionStorage // raw 是底层会话存储
}

// sessionFromGWS 包装底层会话；ss 为 nil 时返回 nil
func sessionFromGWS(ss gws.SessionStorage) Session {
	if ss == nil {
		return nil
	}
	return &sessionAdapter{raw: ss}
}

// Load 按键取值
func (s *sessionAdapter) Load(key string) (any, bool) {
	return s.raw.Load(key)
}

// Store 存键值
func (s *sessionAdapter) Store(key string, value any) {
	s.raw.Store(key, value)
}

// Delete 删键
func (s *sessionAdapter) Delete(key string) {
	s.raw.Delete(key)
}

// Len 返回条目数
func (s *sessionAdapter) Len() int {
	return s.raw.Len()
}

const connSessionKey = "__yomi_ws_conn__"
