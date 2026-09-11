// Package ws 提供实例内 ConnID → 连接表（未导出）
//
// @author zwei
package ws

import (
	"context"
	"sync"

	"gitee.com/hq_project/yomi-boot/errors"
)

// hub 是进程内连接注册表；每 Server 实例一个
type hub struct {
	mu    sync.RWMutex
	conns map[string]*connImpl
}

// newHub 创建空连接表
func newHub() *hub {
	return &hub{conns: make(map[string]*connImpl)}
}

// get 按 id 取连接
func (h *hub) get(id string) (*connImpl, bool) {
	if h == nil {
		return nil, false
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	c, ok := h.conns[id]
	return c, ok
}

// len 返回注册数量
func (h *hub) len() int {
	if h == nil {
		return 0
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.conns)
}

// snapshot 复制当前连接列表
func (h *hub) snapshot() []*connImpl {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	list := make([]*connImpl, 0, len(h.conns))
	for _, c := range h.conns {
		list = append(list, c)
	}
	return list
}

// putLocked 登记连接（调用方须已持有 mu）
func (h *hub) putLocked(id string, c *connImpl) {
	h.conns[id] = c
}

// deleteLocked 按身份删除（调用方须已持有 mu）
func (h *hub) deleteLocked(id string, c *connImpl) {
	if cur, ok := h.conns[id]; ok && (c == nil || cur == c) {
		delete(h.conns, id)
	}
}

// unregister 移除 id；若当前仍是 c 才删除
func (h *hub) unregister(id string, c *connImpl) {
	if h == nil || id == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.deleteLocked(id, c)
}

// broadcast 向全部连接发送同一载荷
func (h *hub) broadcast(typ MsgType, data []byte) error {
	if h == nil {
		return errors.Sentinel(ErrInvalidArgument)
	}
	list := h.snapshot()
	ctx := context.Background()
	var firstErr error
	for _, c := range list {
		if err := c.write(ctx, typ, data); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// takeAll 清空表并返回原有连接（调用方须已持有 mu）
func (h *hub) takeAllLocked() []*connImpl {
	list := make([]*connImpl, 0, len(h.conns))
	for _, c := range h.conns {
		list = append(list, c)
	}
	h.conns = make(map[string]*connImpl)
	return list
}
