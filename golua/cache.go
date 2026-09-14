// Package golua 脚本内容缓存接口（包内）
//
// @author zwei
package golua

import "context"

// scriptCache 脚本快照缓存（由 Config.CacheBackend 在库内装配）；与进程内 Proto LRU 无关
type scriptCache interface {
	// Get 按 ID 读取；不存在返回 ErrNotFound
	Get(ctx context.Context, id string) (*Script, error)
	// GetWithGen 读取脚本并附带共享代际；genOK=false 表示 gen 不可用（仍可能返回 script）
	GetWithGen(ctx context.Context, id string) (script *Script, gen uint64, genOK bool, err error)
	// CurrentGen 返回共享代际；ok=false 表示本后端不追踪代际
	CurrentGen(ctx context.Context) (gen uint64, ok bool, err error)
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
	// ReplaceAllFrom 在写互斥内先 load 再全量替换
	ReplaceAllFrom(ctx context.Context, load func(context.Context) ([]*Script, error)) error
	// UpsertFrom 在写互斥内 load 单脚本：成功则 Put；ErrNotFound 且 removeOnNotFound 则 Remove
	UpsertFrom(ctx context.Context, id string, removeOnNotFound bool, load func(context.Context) (*Script, error)) error
}
