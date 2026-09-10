// Package ws 转发常用的 wsbase 类型与常量，便于业务只依赖本包
//
// @author zwei
package ws

import "gitee.com/hq_project/yomi-boot/ws/wsbase"

type (
	// Opcode 表示 WebSocket 数据帧操作码
	Opcode = wsbase.Opcode
	// CloseCode 表示关闭帧状态码
	CloseCode = wsbase.CloseCode
	// Conn 是底层连接门面
	Conn = wsbase.Conn
	// Message 是已接收的完整消息视图；用毕须 Close
	Message = wsbase.Message
	// Hub 是连接表
	Hub = wsbase.Hub
	// Session 是连接级会话存储
	Session = wsbase.Session
	// WriteOption 是 WriteReader 单次选项
	WriteOption = wsbase.WriteOption
)

const (
	OpcodeText   = wsbase.OpcodeText
	OpcodeBinary = wsbase.OpcodeBinary

	CloseNormalClosure           = wsbase.CloseNormalClosure
	CloseGoingAway               = wsbase.CloseGoingAway
	CloseProtocolError           = wsbase.CloseProtocolError
	CloseUnsupportedData         = wsbase.CloseUnsupportedData
	CloseNoStatusRcvd            = wsbase.CloseNoStatusRcvd
	CloseAbnormalClosure         = wsbase.CloseAbnormalClosure
	CloseInvalidFramePayloadData = wsbase.CloseInvalidFramePayloadData
	ClosePolicyViolation         = wsbase.ClosePolicyViolation
	CloseMessageTooBig           = wsbase.CloseMessageTooBig
	CloseMandatoryExtension      = wsbase.CloseMandatoryExtension
	CloseInternalServerErr       = wsbase.CloseInternalServerErr
	CloseTLSHandshake            = wsbase.CloseTLSHandshake

	DefaultChunkSize = wsbase.DefaultChunkSize
	MaxChunkSize     = wsbase.MaxChunkSize
)

// WithChunkSize 指定本次 WriteReader 的分片大小（字节）
var WithChunkSize = wsbase.WithChunkSize
