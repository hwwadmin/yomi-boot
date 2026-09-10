// Package wsbase 定义 Opcode、关闭码与消息视图
//
// @author zwei
package wsbase

import "github.com/lxzan/gws"

// Opcode 表示 WebSocket 数据帧操作码
type Opcode = gws.Opcode

// 数据帧操作码常量（与 gws 对齐）
const (
	OpcodeText   = gws.OpcodeText
	OpcodeBinary = gws.OpcodeBinary
)

// CloseCode 表示关闭帧状态码
type CloseCode uint16

// 常用关闭码
const (
	CloseNormalClosure           CloseCode = 1000
	CloseGoingAway               CloseCode = 1001
	CloseProtocolError           CloseCode = 1002
	CloseUnsupportedData         CloseCode = 1003
	CloseNoStatusRcvd            CloseCode = 1005
	CloseAbnormalClosure         CloseCode = 1006
	CloseInvalidFramePayloadData CloseCode = 1007
	ClosePolicyViolation         CloseCode = 1008
	CloseMessageTooBig           CloseCode = 1009
	CloseMandatoryExtension      CloseCode = 1010
	CloseInternalServerErr       CloseCode = 1011
	CloseTryAgainLater           CloseCode = 1013 // CloseTryAgainLater 服务端过载/达连接上限，客户端可稍后重试
	CloseTLSHandshake            CloseCode = 1015
)

// Message 表示一条已接收的完整消息视图；用毕须 Close 以回收缓冲
type Message interface {
	// Opcode 返回消息操作码
	Opcode() Opcode
	// Bytes 返回消息载荷；大负载场景注意内存占用
	Bytes() []byte
	// Len 返回载荷长度
	Len() int
	// Close 回收底层缓冲
	Close() error
}

// messageView 包装 gws.Message 为门面 Message
type messageView struct {
	raw *gws.Message // raw 是底层消息
}

// Opcode 返回消息操作码
func (m *messageView) Opcode() Opcode {
	if m == nil || m.raw == nil {
		return 0
	}
	return m.raw.Opcode
}

// Bytes 返回消息载荷
func (m *messageView) Bytes() []byte {
	if m == nil || m.raw == nil {
		return nil
	}
	return m.raw.Bytes()
}

// Len 返回载荷长度
func (m *messageView) Len() int {
	if m == nil || m.raw == nil || m.raw.Data == nil {
		return 0
	}
	return m.raw.Data.Len()
}

// Close 回收底层缓冲
func (m *messageView) Close() error {
	if m == nil || m.raw == nil {
		return nil
	}
	err := m.raw.Close()
	m.raw = nil
	return err
}
