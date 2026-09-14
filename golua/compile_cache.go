// Package golua 进程内 Proto LRU
//
// @author zwei
package golua

import (
	"container/list"
	"context"
	"strings"
	"sync"

	"golang.org/x/sync/singleflight"

	lua "github.com/yuin/gopher-lua"
	"github.com/yuin/gopher-lua/parse"
)

// protoCacheKey 编译缓存键：脚本 id + 源码哈希
type protoCacheKey struct {
	id          string
	contentHash string
}

// protoCacheEntry LRU 节点
type protoCacheEntry struct {
	key   protoCacheKey
	proto *lua.FunctionProto
}

// protoCache FunctionProto LRU
type protoCache struct {
	mu       sync.Mutex
	capacity int
	ll       *list.List
	items    map[protoCacheKey]*list.Element
	sf       singleflight.Group
}

// newProtoCache 创建 Proto 缓存
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

// getOrCompile 命中返回缓存 Proto，否则编译并写入 LRU
func (c *protoCache) getOrCompile(ctx context.Context, id, contentHash, source string) (*lua.FunctionProto, error) {
	key := protoCacheKey{id: id, contentHash: contentHash}
	if proto := c.lookup(key); proto != nil {
		return proto, nil
	}

	flightKey := id + "\x00" + contentHash
	type flightResult struct {
		proto *lua.FunctionProto
		err   error
	}
	ch := make(chan flightResult, 1)
	go func() {
		v, err, _ := c.sf.Do(flightKey, func() (any, error) {
			if proto := c.lookup(key); proto != nil {
				return proto, nil
			}
			proto, err := compileSource(id, source)
			if err != nil {
				return nil, err
			}
			c.store(key, proto)
			return proto, nil
		})
		if err != nil {
			ch <- flightResult{err: err}
			return
		}
		ch <- flightResult{proto: v.(*lua.FunctionProto)}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		return r.proto, r.err
	}
}

// lookup 读取缓存；命中时提升 LRU
func (c *protoCache) lookup(key protoCacheKey) *lua.FunctionProto {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil
	}
	c.ll.MoveToFront(el)
	return el.Value.(*protoCacheEntry).proto
}

// store 写入 LRU
func (c *protoCache) store(key protoCacheKey, proto *lua.FunctionProto) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.ll.MoveToFront(el)
		el.Value.(*protoCacheEntry).proto = proto
		return
	}
	el := c.ll.PushFront(&protoCacheEntry{key: key, proto: proto})
	c.items[key] = el
	for c.ll.Len() > c.capacity {
		c.evictOldest()
	}
}

// removeIDs 按脚本 id 删除相关编译缓存
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

// clear 清空编译缓存
func (c *protoCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ll.Init()
	c.items = make(map[protoCacheKey]*list.Element)
}

// evictOldest 淘汰最久未使用条目；调用方须持锁
func (c *protoCache) evictOldest() {
	el := c.ll.Back()
	if el == nil {
		return
	}
	c.ll.Remove(el)
	ent := el.Value.(*protoCacheEntry)
	if cur, ok := c.items[ent.key]; ok && cur == el {
		delete(c.items, ent.key)
	}
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
