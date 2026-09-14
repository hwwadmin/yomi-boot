// Package golua 内存脚本缓存
//
// @author zwei
package golua

import (
	"context"
	"sync"

	"gitee.com/hq_project/yomi-boot/errors"
)

var _ ScriptCache = (*MemoryScriptCache)(nil)

// MemoryScriptCache 进程内脚本快照缓存（单机）
type MemoryScriptCache struct {
	mu      sync.RWMutex
	ready   bool
	scripts map[string]*Script
}

// newMemoryScriptCache 创建空的内存脚本缓存
func newMemoryScriptCache() *MemoryScriptCache {
	return &MemoryScriptCache{
		scripts: make(map[string]*Script),
	}
}

// Get 按 ID 读取
func (c *MemoryScriptCache) Get(_ context.Context, id string) (*Script, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s, ok := c.scripts[id]
	if !ok {
		return nil, errors.Sentinel(ErrNotFound)
	}
	cp := *s
	return &cp, nil
}

// Has 判断是否存在
func (c *MemoryScriptCache) Has(_ context.Context, id string) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.scripts[id]
	return ok, nil
}

// Ready 已灌库且非空
func (c *MemoryScriptCache) Ready(_ context.Context) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ready && len(c.scripts) > 0, nil
}

// Put 写入单脚本
func (c *MemoryScriptCache) Put(_ context.Context, script *Script) error {
	if script == nil {
		return errors.New("golua: 脚本不能为空")
	}
	cp := *script
	if err := cp.normalize(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.scripts[cp.ID] = &cp
	c.ready = true
	return nil
}

// Remove 删除单脚本
func (c *MemoryScriptCache) Remove(_ context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.scripts, id)
	return nil
}

// ReplaceAll 全量替换
func (c *MemoryScriptCache) ReplaceAll(_ context.Context, scripts []*Script) error {
	next := make(map[string]*Script, len(scripts))
	for _, sc := range scripts {
		if sc == nil {
			continue
		}
		cp := *sc
		if err := cp.normalize(); err != nil {
			return err
		}
		if _, dup := next[cp.ID]; dup {
			return errors.Newf("golua: 脚本 id 重复: %s", cp.ID)
		}
		next[cp.ID] = &cp
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.scripts = next
	c.ready = true
	return nil
}

// ReplaceAllFrom 先 load 再全量替换（单机由 StoreLoad.mu 串行，此处无需额外锁跨越 load）
func (c *MemoryScriptCache) ReplaceAllFrom(ctx context.Context, load func(context.Context) ([]*Script, error)) error {
	if load == nil {
		return errors.New("golua: load 不能为空")
	}
	scripts, err := load(ctx)
	if err != nil {
		return err
	}
	return c.ReplaceAll(ctx, scripts)
}
