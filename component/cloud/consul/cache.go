// Package consul 提供服务实例本地缓存：提供进程内一级缓存的读取、写入与陈旧兜底
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

// serviceCache 进程内一级缓存，缓存健康实例列表。
// 过期条目不物理删除，保留供发现失败时的陈旧兜底（getStale）；
// 物理删除只发生在被新结果覆盖或显式 delete，键集合有限（服务名），无泄漏风险。
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

// get 从缓存读取未过期的服务实例列表，过期或不存在时返回 false（不删除条目）
func (c *serviceCache) get(serviceName string) ([]ServiceInstance, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.items[serviceName]
	if !ok || !time.Now().Before(entry.expiresAt) {
		return nil, false
	}
	out := make([]ServiceInstance, len(entry.instances))
	copy(out, entry.instances)
	return out, true
}

// getStale 返回陈旧但仍在兜底窗口内的实例列表；窗口从条目过期时刻起算（RFC 5861 语义），
// staleWindow <= 0 表示关闭陈旧兜底
func (c *serviceCache) getStale(serviceName string, staleWindow time.Duration) ([]ServiceInstance, bool) {
	if staleWindow <= 0 {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.items[serviceName]
	if !ok || !time.Now().Before(entry.expiresAt.Add(staleWindow)) {
		return nil, false
	}
	out := make([]ServiceInstance, len(entry.instances))
	copy(out, entry.instances)
	return out, true
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

// delete 删除指定服务的缓存条目（含陈旧条目）
func (c *serviceCache) delete(serviceName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, serviceName)
}

// discoveryNegativeTTL 是发现失败负缓存与后台再验证探测限频的默认窗口
const discoveryNegativeTTL = time.Second

// negativeCache 短暂缓存最近一次发现失败的结果：断连期间窗口内的请求直接拿到
// 上次失败而不再探测 Agent；后台再验证也以它限频，探测频率不超过每窗口一次
type negativeCache struct {
	mu    sync.Mutex
	ttl   time.Duration
	items map[string]negativeEntry
}

// negativeEntry 是单服务的失败缓存项
type negativeEntry struct {
	err   error
	until time.Time
}

// newNegativeCache 创建发现失败负缓存
func newNegativeCache(ttl time.Duration) *negativeCache {
	return &negativeCache{
		ttl:   ttl,
		items: make(map[string]negativeEntry),
	}
}

// get 返回窗口内的最近一次失败；过期条目顺手清理
func (c *negativeCache) get(serviceName string) (error, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.items[serviceName]
	if !ok {
		return nil, false
	}
	if !time.Now().Before(entry.until) {
		delete(c.items, serviceName)
		return nil, false
	}
	return entry.err, true
}

// set 记录一次失败并刷新窗口
func (c *negativeCache) set(serviceName string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[serviceName] = negativeEntry{err: err, until: time.Now().Add(c.ttl)}
}
