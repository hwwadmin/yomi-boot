// Package golua Store 对外接口
//
// @author zwei
package golua

import "context"

// StoreLoadAdapter 业务脚本数据源
type StoreLoadAdapter interface {
	// Load 返回全部脚本快照
	Load(ctx context.Context) ([]*Script, error)
	// LoadByID 按 ID 拉取脚本；不存在返回 ErrNotFound
	LoadByID(ctx context.Context, id string) (*Script, error)
}

// StoreLoad 脚本缓存灌入与热更新
type StoreLoad interface {
	// Refresh 全量灌入；缓存已 Ready 则跳过
	Refresh(ctx context.Context) error
	// RefreshForce 强制全量回源并覆盖缓存
	RefreshForce(ctx context.Context) error
	// RefreshScript 单脚本灌入；已存在则跳过
	RefreshScript(ctx context.Context, id string) error
	// RefreshScriptForce 强制单脚本回源；NotFound 时删除缓存
	RefreshScriptForce(ctx context.Context, id string) error
}

// StoreQuery 脚本快照只读查询
type StoreQuery interface {
	// Get 按 ID 读快照；不存在返回 ErrNotFound
	Get(ctx context.Context, id string) (*Script, error)
}

// ScriptInvalidator 本地编译缓存与执行池失效
type ScriptInvalidator interface {
	// InvalidateScripts 丢弃指定脚本的 Proto 并提升池世代
	InvalidateScripts(ids ...string)
	// InvalidateAllScripts 清空 Proto 并提升池世代
	InvalidateAllScripts()
	// ObserveGeneration 记录已对齐的共享代际
	ObserveGeneration(gen uint64)
}
