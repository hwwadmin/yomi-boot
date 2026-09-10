// Package ws 将门面 MsgType 与传输层 Opcode / 写入互转
//
// @author zwei
package ws

import (
	"context"

	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/ws/wsbase"
)

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
