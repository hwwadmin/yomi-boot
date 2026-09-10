// Package ws 提供实例级连接表与广播
//
// @author zwei
package ws

import (
	"context"
	"sync"

	"gitee.com/hq_project/yomi-boot/errors"

	"github.com/lxzan/gws"
)

// Hub 是进程内连接注册表；建议每 Server 实例一个，非全局单例
type Hub struct {
	mu    sync.RWMutex
	conns map[string]Conn
}

// NewHub 创建空连接表
func NewHub() *Hub {
	return &Hub{conns: make(map[string]Conn)}
}

// Register 按 id 注册连接；同 id 覆盖旧连接（不自动关闭旧连接）
func (h *Hub) Register(id string, c Conn) {
	if h == nil || id == "" || c == nil {
		return
	}
	h.mu.Lock()
	h.conns[id] = c
	h.mu.Unlock()
}

// Unregister 移除 id；若当前仍是 c 才删除（避免误删新连接）
func (h *Hub) Unregister(id string, c Conn) {
	if h == nil || id == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if cur, ok := h.conns[id]; ok && (c == nil || cur == c) {
		delete(h.conns, id)
	}
}

// Get 按 id 取连接
func (h *Hub) Get(id string) (Conn, bool) {
	if h == nil {
		return nil, false
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	c, ok := h.conns[id]
	return c, ok
}

// Len 返回注册数量
func (h *Hub) Len() int {
	if h == nil {
		return 0
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.conns)
}

// BroadcastBinary 向所有连接发送同一二进制载荷；单连接失败不中断其余
func (h *Hub) BroadcastBinary(payload []byte) error {
	return h.broadcast(OpcodeBinary, payload)
}

// BroadcastText 向所有连接发送同一文本载荷；单连接失败不中断其余
func (h *Hub) BroadcastText(payload []byte) error {
	return h.broadcast(OpcodeText, payload)
}

// broadcast 向 Hub 内全部连接发送同一载荷
func (h *Hub) broadcast(opcode Opcode, payload []byte) error {
	if h == nil {
		return errors.Sentinel(ErrInvalidArgument)
	}
	h.mu.RLock()
	list := make([]Conn, 0, len(h.conns))
	for _, c := range h.conns {
		list = append(list, c)
	}
	h.mu.RUnlock()

	var firstErr error
	b := gws.NewBroadcaster(opcode, payload)
	defer func() { _ = b.Close() }()
	for _, c := range list {
		native, ok := c.(NativeConn)
		if !ok || native.Native() == nil {
			if err := writeFallback(c, opcode, payload); err != nil && firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := b.Broadcast(native.Native(), nil); err != nil && firstErr == nil {
			firstErr = errors.Wrap(err, "ws Hub 广播失败")
		}
	}
	return firstErr
}

// writeFallback 在无法走 Broadcaster 时回退到门面写入
func writeFallback(c Conn, opcode Opcode, payload []byte) error {
	if opcode == OpcodeText {
		return c.WriteText(context.Background(), payload)
	}
	return c.WriteBinary(context.Background(), payload)
}

// CloseAll 关闭全部连接并清空表
func (h *Hub) CloseAll(code CloseCode, reason string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	list := make([]Conn, 0, len(h.conns))
	for _, c := range h.conns {
		list = append(list, c)
	}
	h.conns = make(map[string]Conn)
	h.mu.Unlock()
	for _, c := range list {
		_ = c.Close(code, reason)
	}
}
