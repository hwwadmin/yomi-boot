// Package ws 定义业务消息处理器
//
// @author zwei
package ws

// Payload 是业务可见的消息载荷；默认 Handler 已关闭底层 Message
type Payload struct {
	Opcode Opcode // Opcode 是消息类型（Text/Binary）
	Data   []byte // Data 是载荷副本，业务可安全持有
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

// takePayload 拷贝载荷并关闭底层消息
func takePayload(msg Message) Payload {
	if msg == nil {
		return Payload{}
	}
	defer msg.Close()
	raw := msg.Bytes()
	data := append([]byte(nil), raw...)
	return Payload{Opcode: msg.Opcode(), Data: data}
}
