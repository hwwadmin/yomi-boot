// Package wsapp 定义业务消息处理器
//
// @author zwei
package wsapp

import "gitee.com/hq_project/yomi-boot/ws"

// Payload 是业务可见的消息载荷；默认 Handler 已关闭底层 ws.Message
type Payload struct {
	Opcode ws.Opcode // Opcode 是消息类型（Text/Binary）
	Data   []byte    // Data 是载荷副本，业务可安全持有
}

// ServerMessageProcessor 服务端业务消息处理器；不接触 ws.Message
type ServerMessageProcessor interface {
	// OnPayload 处理一条业务载荷
	OnPayload(ctx SessionContext, p Payload) error
}

// ClientMessageProcessor 客户端业务消息处理器；不接触 ws.Message
type ClientMessageProcessor interface {
	// OnPayload 处理一条业务载荷
	OnPayload(ctx PeerContext, p Payload) error
}

// ServerLifecycle 可选：需要感知连接启停时由 Processor 实现
type ServerLifecycle interface {
	// OnConnect 连接已建立
	OnConnect(ctx SessionContext)
	// OnDisconnect 连接已结束
	OnDisconnect(connID string, err error)
}

// ClientLifecycle 可选：需要感知连接启停时由 Processor 实现
type ClientLifecycle interface {
	// OnConnect 出站已连通
	OnConnect(ctx PeerContext)
	// OnDisconnect 连接已结束
	OnDisconnect(err error)
}

// takePayload 拷贝载荷并关闭底层消息
func takePayload(msg ws.Message) Payload {
	if msg == nil {
		return Payload{}
	}
	defer msg.Close()
	raw := msg.Bytes()
	data := append([]byte(nil), raw...)
	return Payload{Opcode: msg.Opcode(), Data: data}
}
