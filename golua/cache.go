// Package golua 脚本内容缓存接口
//
// @author zwei
package golua

import "context"

// ScriptCache 脚本快照缓存（由 Config.CacheBackend 在库内装配）；与进程内 Proto LRU 无关
type ScriptCache interface {
	// Get 按 ID 读取；不存在返回 ErrNotFound
	Get(ctx context.Context, id string) (*Script, error)
	// Has 判断 id 是否在缓存中
	Has(ctx context.Context, id string) (bool, error)
	// Ready 普通全量 Refresh 判空：已初始化且 index 非空
	Ready(ctx context.Context) (bool, error)
	// Put 写入/覆盖单脚本并维护 index
	Put(ctx context.Context, script *Script) error
	// Remove 删除单脚本并从 index 移除
	Remove(ctx context.Context, id string) error
	// ReplaceAll 用全量快照替换缓存并 reconcile 孤儿
	ReplaceAll(ctx context.Context, scripts []*Script) error
	// ReplaceAllFrom 在写互斥内先 load 再全量替换（Redis 与 distlock 同一把锁，避免并发 Force 旧快照后写覆盖）
	ReplaceAllFrom(ctx context.Context, load func(context.Context) ([]*Script, error)) error
}
