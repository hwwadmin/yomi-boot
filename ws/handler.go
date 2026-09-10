// Package ws 定义 Handler、默认实现与会话 Context
//
// @author zwei
package ws

import (
	"context"
	"io"

	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/logger"
	"gitee.com/hq_project/yomi-boot/ws/wsbase"
)

// ServerHandler 是服务端会话回调；可通过 SessionContext.ConnID 做框架级推送
type ServerHandler interface {
	// OnConnect 连接已建立，ConnID 已由框架生成并登记
	OnConnect(ctx SessionContext)
	// OnDisconnect 连接结束
	OnDisconnect(connID string, err error)
	// OnMessage 收到完整消息；用毕须 msg.Close()
	OnMessage(ctx SessionContext, msg wsbase.Message)
}

// ClientHandler 是客户端会话回调；不涉及 ConnID
type ClientHandler interface {
	// OnConnect 出站已连通
	OnConnect(ctx PeerContext)
	// OnDisconnect 连接结束
	OnDisconnect(err error)
	// OnMessage 收到完整消息；用毕须 msg.Close()
	OnMessage(ctx PeerContext, msg wsbase.Message)
}

// DefaultServerHandler 是默认服务端 Handler：启停打日志，OnMessage 将消息交 Processor 消费。
// 自定义 Handler 可嵌入本类型以复用 Processor 消费逻辑。
type DefaultServerHandler struct {
	Processor ServerMessageProcessor // Processor 是业务消息处理器；nil 时丢弃消息
}

// OnConnect 默认打印连接建立日志
func (h DefaultServerHandler) OnConnect(ctx SessionContext) {
	logger.Info(ctx.Context(), "ws: 服务端连接已建立", "conn_id", ctx.ConnID())
}

// OnDisconnect 默认打印连接结束日志
func (h DefaultServerHandler) OnDisconnect(connID string, err error) {
	logger.InfoWithoutCtx("ws: 服务端连接已结束", "conn_id", connID, "error", err)
}

// OnMessage 关闭原始消息后交 Processor；Processor 为 nil 则丢弃
func (h DefaultServerHandler) OnMessage(ctx SessionContext, msg wsbase.Message) {
	p := takePayload(msg)
	if h.Processor == nil {
		return
	}
	_ = h.Processor.OnPayload(ctx, p)
}

// DefaultClientHandler 是默认客户端 Handler（语义同 DefaultServerHandler）
type DefaultClientHandler struct {
	Processor ClientMessageProcessor // Processor 是业务消息处理器；nil 时丢弃消息
}

// OnConnect 默认打印出站连通日志
func (h DefaultClientHandler) OnConnect(ctx PeerContext) {
	logger.Info(ctx.Context(), "ws: 客户端已连通")
}

// OnDisconnect 默认打印连接结束日志
func (h DefaultClientHandler) OnDisconnect(err error) {
	logger.InfoWithoutCtx("ws: 客户端连接已结束", "error", err)
}

// OnMessage 关闭原始消息后交 Processor；Processor 为 nil 则丢弃
func (h DefaultClientHandler) OnMessage(ctx PeerContext, msg wsbase.Message) {
	p := takePayload(msg)
	if h.Processor == nil {
		return
	}
	_ = h.Processor.OnPayload(ctx, p)
}

// NopServerHandler 空服务端 Handler（丢弃消息）
type NopServerHandler struct{}

// OnConnect 空实现
func (NopServerHandler) OnConnect(SessionContext) {}

// OnDisconnect 空实现
func (NopServerHandler) OnDisconnect(string, error) {}

// OnMessage 关闭消息后返回
func (NopServerHandler) OnMessage(_ SessionContext, msg wsbase.Message) {
	if msg != nil {
		_ = msg.Close()
	}
}

// NopClientHandler 空客户端 Handler（丢弃消息）
type NopClientHandler struct{}

// OnConnect 空实现
func (NopClientHandler) OnConnect(PeerContext) {}

// OnDisconnect 空实现
func (NopClientHandler) OnDisconnect(error) {}

// OnMessage 关闭消息后返回
func (NopClientHandler) OnMessage(_ PeerContext, msg wsbase.Message) {
	if msg != nil {
		_ = msg.Close()
	}
}

// Sender 是面向当前连接的发送能力
type Sender interface {
	// SendText 发送文本
	SendText(data []byte) error
	// SendBinary 发送二进制
	SendBinary(data []byte) error
	// SendReader 分片发送
	SendReader(opcode wsbase.Opcode, r io.Reader, opts ...wsbase.WriteOption) error
}

// PeerContext 是客户端侧当前连接视图（无 ConnID）
type PeerContext interface {
	Sender
	// Context 返回关联 context.Context
	Context() context.Context
	// Session 返回连接级会话存储
	Session() wsbase.Session
	// Conn 逃生舱
	Conn() wsbase.Conn
}

// SessionContext 是服务端侧会话视图（含框架 ConnID）
type SessionContext interface {
	PeerContext
	// ConnID 返回服务端生成的框架会话 id
	ConnID() string
}

// peerContext 是 PeerContext 实现
type peerContext struct {
	ctx  context.Context
	conn wsbase.Conn
}

// Context 返回关联 context
func (c *peerContext) Context() context.Context {
	if c == nil || c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

// Session 返回会话存储
func (c *peerContext) Session() wsbase.Session {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Session()
}

// SendText 发送文本
func (c *peerContext) SendText(data []byte) error {
	if c == nil || c.conn == nil {
		return errors.Sentinel(ErrNotConnected)
	}
	return c.conn.WriteText(c.Context(), data)
}

// SendBinary 发送二进制
func (c *peerContext) SendBinary(data []byte) error {
	if c == nil || c.conn == nil {
		return errors.Sentinel(ErrNotConnected)
	}
	return c.conn.WriteBinary(c.Context(), data)
}

// SendReader 分片发送
func (c *peerContext) SendReader(opcode wsbase.Opcode, r io.Reader, opts ...wsbase.WriteOption) error {
	if c == nil || c.conn == nil {
		return errors.Sentinel(ErrNotConnected)
	}
	return c.conn.WriteReader(c.Context(), opcode, r, opts...)
}

// Conn 返回底层连接
func (c *peerContext) Conn() wsbase.Conn {
	if c == nil {
		return nil
	}
	return c.conn
}

// sessionContext 是 SessionContext 实现
type sessionContext struct {
	peerContext
	connID string
}

// ConnID 返回框架会话 id
func (c *sessionContext) ConnID() string {
	if c == nil {
		return ""
	}
	return c.connID
}
