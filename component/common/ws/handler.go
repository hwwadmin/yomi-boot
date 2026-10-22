// Package ws 定义 Handler 与会话 Context
//
// @author zwei
package ws

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/common/logger"
	"gitee.com/hq_project/yomi-boot/core/errors"
)

// ServerHandler 是服务端会话回调；可通过 SessionContext.ConnID 做框架级推送
type ServerHandler interface {
	// OnConnect 连接已建立，ConnID 已由框架生成并登记
	OnConnect(ctx SessionContext)
	// OnDisconnect 连接结束
	OnDisconnect(connID string, err error)
	// OnPayload 收到业务载荷
	OnPayload(ctx SessionContext, p Payload)
}

// ClientHandler 是客户端会话回调；不涉及 ConnID
type ClientHandler interface {
	// OnConnect 出站已连通
	OnConnect(ctx PeerContext)
	// OnDisconnect 连接结束
	OnDisconnect(err error)
	// OnPayload 收到业务载荷
	OnPayload(ctx PeerContext, p Payload)
}

// DefaultServerHandler 是默认服务端 Handler：启停打日志，OnPayload 将消息交 Processor。
// 自定义 Handler 可嵌入本类型以复用 Processor 消费逻辑。
type DefaultServerHandler struct {
	Name      string                 // Name 实例名，写入日志；可空
	Processor ServerMessageProcessor // Processor 业务消息处理器；nil 时丢弃消息
}

// OnConnect 默认打印连接建立日志
func (h DefaultServerHandler) OnConnect(ctx SessionContext) {
	logger.Info(ctx.Context(), "ws: 服务端连接已建立", "name", h.Name, "conn_id", ctx.ConnID())
}

// OnDisconnect 默认打印连接结束日志
func (h DefaultServerHandler) OnDisconnect(connID string, err error) {
	logger.InfoWithoutCtx("ws: 服务端连接已结束", "name", h.Name, "conn_id", connID, "error", err)
}

// OnPayload 交 Processor；Processor 错误记日志
func (h DefaultServerHandler) OnPayload(ctx SessionContext, p Payload) {
	if h.Processor == nil {
		return
	}
	if err := h.Processor.OnPayload(ctx, p); err != nil {
		logger.Error(ctx.Context(), "ws: Processor.OnPayload 失败", "name", h.Name, "conn_id", ctx.ConnID(), "error", err)
	}
}

// DefaultClientHandler 是默认客户端 Handler（语义同 DefaultServerHandler）
type DefaultClientHandler struct {
	Name      string                 // Name 实例名，写入日志；可空
	Processor ClientMessageProcessor // Processor 业务消息处理器；nil 时丢弃消息
}

// OnConnect 默认打印出站连通日志
func (h DefaultClientHandler) OnConnect(ctx PeerContext) {
	logger.Info(ctx.Context(), "ws: 客户端已连通", "name", h.Name)
}

// OnDisconnect 默认打印连接结束日志
func (h DefaultClientHandler) OnDisconnect(err error) {
	logger.InfoWithoutCtx("ws: 客户端连接已结束", "name", h.Name, "error", err)
}

// OnPayload 交 Processor；Processor 错误记日志
func (h DefaultClientHandler) OnPayload(ctx PeerContext, p Payload) {
	if h.Processor == nil {
		return
	}
	if err := h.Processor.OnPayload(ctx, p); err != nil {
		logger.Error(ctx.Context(), "ws: Processor.OnPayload 失败", "name", h.Name, "error", err)
	}
}

// NopServerHandler 空服务端 Handler
type NopServerHandler struct{}

// OnConnect 空实现
func (NopServerHandler) OnConnect(SessionContext) {}

// OnDisconnect 空实现
func (NopServerHandler) OnDisconnect(string, error) {}

// OnPayload 空实现
func (NopServerHandler) OnPayload(SessionContext, Payload) {}

// NopClientHandler 空客户端 Handler
type NopClientHandler struct{}

// OnConnect 空实现
func (NopClientHandler) OnConnect(PeerContext) {}

// OnDisconnect 空实现
func (NopClientHandler) OnDisconnect(error) {}

// OnPayload 空实现
func (NopClientHandler) OnPayload(PeerContext, Payload) {}

// Sender 是面向当前连接的发送能力（入队语义与 Client/Server.Send 一致）
type Sender interface {
	// Send 按类型入队发送一条消息
	Send(typ MsgType, data []byte) error
}

// PeerContext 是客户端侧当前连接视图（无 ConnID）
type PeerContext interface {
	Sender
	// Context 返回关联 context.Context
	Context() context.Context
	// Session 返回连接级会话存储
	Session() Session
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
	conn *connImpl
}

// Context 返回关联 context
func (c *peerContext) Context() context.Context {
	if c == nil || c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

// Session 返回会话存储
func (c *peerContext) Session() Session {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.session
}

// Send 按类型入队发送
func (c *peerContext) Send(typ MsgType, data []byte) error {
	if c == nil || c.conn == nil {
		return errors.Sentinel(ErrNotConnected)
	}
	return c.conn.enqueue(typ, data)
}

// sessionContext 是 SessionContext 实现
type sessionContext struct {
	peerContext
}

// ConnID 返回框架会话 id
func (c *sessionContext) ConnID() string {
	if c == nil || c.conn == nil {
		return ""
	}
	return c.conn.connID
}
