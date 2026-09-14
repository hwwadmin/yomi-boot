// Package golua 脚本快照缓存接口
//
// @author zwei
package golua

import "context"

// scriptCache 脚本快照缓存
type scriptCache interface {
	// Get 按 ID 读取；不存在返回 ErrNotFound
	Get(ctx context.Context, id string) (*Script, error)
	// GetWithGen 读取脚本及共享代际；genOK 为 false 表示无有效代际
	GetWithGen(ctx context.Context, id string) (script *Script, gen uint64, genOK bool, err error)
	// CurrentGen 返回共享代际；ok 为 false 表示无代际
	CurrentGen(ctx context.Context) (gen uint64, ok bool, err error)
	// Has 判断是否存在
	Has(ctx context.Context, id string) (bool, error)
	// Ready 是否已灌库且非空
	Ready(ctx context.Context) (bool, error)
	// Put 写入单脚本
	Put(ctx context.Context, script *Script) error
	// Remove 删除单脚本
	Remove(ctx context.Context, id string) error
	// ReplaceAll 全量替换
	ReplaceAll(ctx context.Context, scripts []*Script) error
	// ReplaceAllFrom 回源后全量替换
	ReplaceAllFrom(ctx context.Context, load func(context.Context) ([]*Script, error)) error
	// UpsertFrom 回源后写入；NotFound 且 removeOnNotFound 时删除
	UpsertFrom(ctx context.Context, id string, removeOnNotFound bool, load func(context.Context) (*Script, error)) error
}
