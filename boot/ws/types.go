// Package ws 定义消息类型、关闭码与会话存储
//
// @author zwei
package ws

import "sync"

// MsgType 是业务可见的消息类型
type MsgType uint8

const (
	// MsgText 文本消息
	MsgText MsgType = 1
	// MsgBinary 二进制消息
	MsgBinary MsgType = 2
)

// Payload 是一条已收齐的业务载荷；Data 归回调方持有
type Payload struct {
	Type MsgType // Type 是业务消息类型
	Data []byte  // Data 是载荷
}

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

// Session 是连接级键值存储，供业务挂私有状态；框架自身不往里写
type Session interface {
	// Load 按键取值
	Load(key string) (value any, ok bool)
	// Store 存键值
	Store(key string, value any)
	// Delete 删键
	Delete(key string)
	// Len 返回条目数
	Len() int
}

// session 是基于 sync.Map 的 Session 实现
type session struct {
	m sync.Map // m 存键值
}

// newSession 创建空会话存储
func newSession() *session {
	return &session{}
}

// Load 按键取值
func (s *session) Load(key string) (any, bool) {
	if s == nil {
		return nil, false
	}
	return s.m.Load(key)
}

// Store 存键值
func (s *session) Store(key string, value any) {
	if s == nil {
		return
	}
	s.m.Store(key, value)
}

// Delete 删键
func (s *session) Delete(key string) {
	if s == nil {
		return
	}
	s.m.Delete(key)
}

// Len 返回条目数
func (s *session) Len() int {
	if s == nil {
		return 0
	}
	n := 0
	s.m.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
}
