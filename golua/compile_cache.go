// Package golua 进程内 Proto LRU（键为 id + 源码 content hash）
//
// @author zwei
package golua

import (
	"container/list"
	"strings"
	"sync"

	lua "github.com/yuin/gopher-lua"
	"github.com/yuin/gopher-lua/parse"
)

// protoCacheKey 编译缓存键（脚本 id + 源码哈希）
type protoCacheKey struct {
	id          string
	contentHash string
}

// protoCacheEntry LRU 节点值
type protoCacheEntry struct {
	key   protoCacheKey
	proto *lua.FunctionProto
}

// protoCache 进程内 FunctionProto LRU
type protoCache struct {
	mu       sync.Mutex
	capacity int
	ll       *list.List
	items    map[protoCacheKey]*list.Element
}

// newProtoCache 创建指定容量的 Proto 缓存
func newProtoCache(capacity int) *protoCache {
	if capacity <= 0 {
		capacity = defaultProtoCacheSize
	}
	return &protoCache{
		capacity: capacity,
		ll:       list.New(),
		items:    make(map[protoCacheKey]*list.Element),
	}
}

// getOrCompile 命中则返回缓存 Proto，否则编译并写入 LRU（contentHash 须与 source 对应）
func (c *protoCache) getOrCompile(id, contentHash, source string) (*lua.FunctionProto, error) {
	key := protoCacheKey{id: id, contentHash: contentHash}
	c.mu.Lock()
	if el, ok := c.items[key]; ok {
		c.ll.MoveToFront(el)
		proto := el.Value.(*protoCacheEntry).proto
		c.mu.Unlock()
		return proto, nil
	}
	c.mu.Unlock()

	proto, err := compileSource(id, source)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.ll.MoveToFront(el)
		return el.Value.(*protoCacheEntry).proto, nil
	}
	el := c.ll.PushFront(&protoCacheEntry{key: key, proto: proto})
	c.items[key] = el
	for c.ll.Len() > c.capacity {
		c.evictOldest()
	}
	return proto, nil
}

// removeIDs 按脚本 id 删除全部 contentHash 的编译缓存
func (c *protoCache) removeIDs(ids ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	want := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			want[id] = struct{}{}
		}
	}
	if len(want) == 0 {
		return
	}
	for key, el := range c.items {
		if _, ok := want[key.id]; ok {
			c.ll.Remove(el)
			delete(c.items, key)
		}
	}
}

// clear 清空全部编译缓存
func (c *protoCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ll.Init()
	c.items = make(map[protoCacheKey]*list.Element)
}

// evictOldest 淘汰最久未使用条目（调用方须持锁）
func (c *protoCache) evictOldest() {
	el := c.ll.Back()
	if el == nil {
		return
	}
	c.ll.Remove(el)
	ent := el.Value.(*protoCacheEntry)
	delete(c.items, ent.key)
}

// compileSource 将 Lua 源码编译为 FunctionProto
func compileSource(name, source string) (*lua.FunctionProto, error) {
	chunk, err := parse.Parse(strings.NewReader(source), name)
	if err != nil {
		return nil, err
	}
	proto, err := lua.Compile(chunk, name)
	if err != nil {
		return nil, err
	}
	return proto, nil
}
