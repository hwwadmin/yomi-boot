// Package ws 定义业务消息类型与处理器
//
// @author zwei
package ws

// MsgType 是门面层消息类型（不暴露 gws Opcode）
type MsgType uint8

const (
	// MsgText 文本消息
	MsgText MsgType = 1
	// MsgBinary 二进制消息
	MsgBinary MsgType = 2
)

// Payload 是业务可见的消息载荷；默认 Handler 已关闭底层 Message
type Payload struct {
	Type MsgType // Type 是业务消息类型
	Data []byte  // Data 是载荷副本，业务可安全持有
}

// ServerMessageProcessor 服务端业务消息处理器；只负责消费 Payload，不接触 Message
type ServerMessageProcessor interface {
	// OnPayload 处理一条业务载荷
	OnPayload(ctx SessionContext, p Payload) error
}

// ClientMessageProcessor 客户端业务消息处理器；只负责消费 Payload，不接触 Message
type ClientMessageProcessor interface {
	// OnPayload 处理一条业务载荷
	OnPayload(ctx PeerContext, p Payload) error
}
