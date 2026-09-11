// Package ws 转发常用的 wsbase 类型与常量，便于业务只依赖本包
//
// @author zwei
package ws

import (
	"time"

	"gitee.com/hq_project/yomi-boot/ws/wsbase"
)

type (
	// CloseCode 表示关闭帧状态码
	CloseCode = wsbase.CloseCode
	// Conn 是底层连接门面
	Conn = wsbase.Conn
	// Message 是已接收的完整消息视图；用毕须 Close（逃生/自定义 Handler 使用）
	Message = wsbase.Message
	// Hub 是连接表
	Hub = wsbase.Hub
	// Session 是连接级会话存储
	Session = wsbase.Session
)

const (
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
	CloseTryAgainLater           = wsbase.CloseTryAgainLater
	CloseTLSHandshake            = wsbase.CloseTLSHandshake

	DefaultWriteMaxPayloadSize = wsbase.DefaultWriteMaxPayloadSize
	MaxWriteMaxPayloadSize     = wsbase.MaxWriteMaxPayloadSize

	DefaultServerHeartbeatInterval = wsbase.DefaultServerHeartbeatInterval
	DefaultClientHeartbeatInterval = wsbase.DefaultClientHeartbeatInterval
	DefaultHeartbeatInterval       = wsbase.DefaultHeartbeatInterval
	DefaultHeartbeatTimeout        = wsbase.DefaultHeartbeatTimeout
	DefaultHandshakeTimeout        = wsbase.DefaultHandshakeTimeout
)

// Duration 返回 d 的指针，便于配置 Heartbeat* 与 MaxConnLifetime* 等 *time.Duration 字段
func Duration(d time.Duration) *time.Duration {
	return wsbase.Duration(d)
}
