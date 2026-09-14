// Package golua 内存脚本缓存
//
// @author zwei
package golua

import (
	"context"
	"sync"

	"gitee.com/hq_project/yomi-boot/errors"
)

var _ scriptCache = (*memoryScriptCache)(nil)

// memoryScriptCache 进程内脚本快照缓存
type memoryScriptCache struct {
	mu      sync.RWMutex
	ready   bool
	scripts map[string]*Script
}

// newMemoryScriptCache 创建内存快照缓存
func newMemoryScriptCache() *memoryScriptCache {
	return &memoryScriptCache{
		scripts: make(map[string]*Script),
	}
}

// Get 按 ID 读取
func (c *memoryScriptCache) Get(_ context.Context, id string) (*Script, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s, ok := c.scripts[id]
	if !ok {
		return nil, errors.Sentinel(ErrNotFound)
	}
	cp := *s
	return &cp, nil
}

// GetWithGen 读取脚本；无共享代际
func (c *memoryScriptCache) GetWithGen(ctx context.Context, id string) (*Script, uint64, bool, error) {
	script, err := c.Get(ctx, id)
	return script, 0, false, err
}

// CurrentGen 无共享代际
func (c *memoryScriptCache) CurrentGen(context.Context) (uint64, bool, error) {
	return 0, false, nil
}

// Has 判断是否存在
func (c *memoryScriptCache) Has(_ context.Context, id string) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.scripts[id]
	return ok, nil
}

// Ready 是否已灌库且非空
func (c *memoryScriptCache) Ready(_ context.Context) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ready && len(c.scripts) > 0, nil
}

// Put 写入单脚本
func (c *memoryScriptCache) Put(_ context.Context, script *Script) error {
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
func (c *memoryScriptCache) Remove(_ context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.scripts, id)
	return nil
}

// ReplaceAll 全量替换
func (c *memoryScriptCache) ReplaceAll(_ context.Context, scripts []*Script) error {
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

// ReplaceAllFrom 回源后全量替换
func (c *memoryScriptCache) ReplaceAllFrom(ctx context.Context, load func(context.Context) ([]*Script, error)) error {
	if load == nil {
		return errors.New("golua: load 不能为空")
	}
	scripts, err := load(ctx)
	if err != nil {
		return err
	}
	return c.ReplaceAll(ctx, scripts)
}

// UpsertFrom 回源后写入；NotFound 且 removeOnNotFound 时删除
func (c *memoryScriptCache) UpsertFrom(ctx context.Context, id string, removeOnNotFound bool, load func(context.Context) (*Script, error)) error {
	if load == nil {
		return errors.New("golua: load 不能为空")
	}
	script, err := load(ctx)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			if !removeOnNotFound {
				return errors.Sentinel(ErrNotFound)
			}
			return c.Remove(ctx, id)
		}
		return err
	}
	if script == nil {
		return errors.New("golua: load 返回空脚本")
	}
	return c.Put(ctx, script)
}
