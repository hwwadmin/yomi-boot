// Package ws 定义业务消息类型与处理器
//
// @author zwei
package ws

import (
	"context"

	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/ws/wsbase"
)

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

// takePayload 拷贝载荷、映射类型并关闭底层消息；无法识别的 opcode 返回 ok=false
func takePayload(msg Message) (Payload, bool) {
	if msg == nil {
		return Payload{}, false
	}
	defer msg.Close()
	typ, ok := msgTypeFromOpcode(msg.Opcode())
	if !ok {
		return Payload{}, false
	}
	raw := msg.Bytes()
	data := append([]byte(nil), raw...)
	return Payload{Type: typ, Data: data}, true
}

// msgTypeFromOpcode 将传输层 Opcode 映射为门面 MsgType
func msgTypeFromOpcode(op wsbase.Opcode) (MsgType, bool) {
	switch op {
	case wsbase.OpcodeText:
		return MsgText, true
	case wsbase.OpcodeBinary:
		return MsgBinary, true
	default:
		return 0, false
	}
}

// writeByType 按 MsgType 写入底层连接
func writeByType(ctx context.Context, conn wsbase.Conn, typ MsgType, data []byte) error {
	if conn == nil {
		return errors.Sentinel(ErrNotConnected)
	}
	switch typ {
	case MsgText:
		return conn.WriteText(ctx, data)
	case MsgBinary:
		return conn.WriteBinary(ctx, data)
	default:
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "不支持的 MsgType")
	}
}
