// Package consul 提供服务实例本地缓存：提供进程内一级缓存的读取、写入与过期自动清理
package consul

import (
	"sync"
	"time"
)

// serviceCacheEntry 缓存项，包含服务实例列表及过期时间
type serviceCacheEntry struct {
	instances []ServiceInstance
	expiresAt time.Time
}

// serviceCache 进程内一级缓存，缓存健康实例列表
type serviceCache struct {
	mu    sync.RWMutex
	ttl   time.Duration
	items map[string]serviceCacheEntry
}

// newServiceCache 创建进程内服务实例缓存
func newServiceCache(ttl time.Duration) *serviceCache {
	return &serviceCache{
		ttl:   ttl,
		items: make(map[string]serviceCacheEntry),
	}
}

// get 从缓存中读取服务实例列表，过期或不存在时返回 false
func (c *serviceCache) get(serviceName string) ([]ServiceInstance, bool) {
	// 读锁查询缓存
	c.mu.RLock()
	entry, ok := c.items[serviceName]
	if !ok {
		c.mu.RUnlock()
		return nil, false
	}
	// 未过期则拷贝返回，避免外部修改影响缓存
	if time.Now().Before(entry.expiresAt) {
		out := make([]ServiceInstance, len(entry.instances))
		copy(out, entry.instances)
		c.mu.RUnlock()
		return out, true
	}
	c.mu.RUnlock()
	// 已过期：加写锁双重检查后删除
	c.mu.Lock()
	if entry, ok := c.items[serviceName]; ok && time.Now().After(entry.expiresAt) {
		delete(c.items, serviceName)
	}
	c.mu.Unlock()
	return nil, false
}

// set 将服务实例列表写入缓存并设置过期时间
func (c *serviceCache) set(serviceName string, instances []ServiceInstance) {
	c.mu.Lock()
	defer c.mu.Unlock()
	stored := make([]ServiceInstance, len(instances))
	copy(stored, instances)
	c.items[serviceName] = serviceCacheEntry{
		instances: stored,
		expiresAt: time.Now().Add(c.ttl),
	}
}
