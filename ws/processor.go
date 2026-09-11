// Package ws 定义业务消息处理器（只负责消费 Payload）
//
// @author zwei
package ws

// ServerMessageProcessor 服务端业务消息处理器；只负责消费 Payload
type ServerMessageProcessor interface {
	// OnPayload 处理一条业务载荷
	OnPayload(ctx SessionContext, p Payload) error
}

// ClientMessageProcessor 客户端业务消息处理器；只负责消费 Payload
type ClientMessageProcessor interface {
	// OnPayload 处理一条业务载荷
	OnPayload(ctx PeerContext, p Payload) error
}
