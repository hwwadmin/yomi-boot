// Package golua Store 三接口定义
//
// @author zwei
package golua

import "context"

// StoreLoadAdapter 业务数据源适配器：只负责拉取脚本原文，不碰缓存
type StoreLoadAdapter interface {
	// Load 返回当前全部脚本快照；失败应整批失败
	Load(ctx context.Context) ([]*Script, error)
	// LoadByID 按 ID 拉取单脚本；不存在返回 ErrNotFound
	LoadByID(ctx context.Context, id string) (*Script, error)
}

// StoreLoad 缓存灌入与热更新；由 golua 实现，暴露给业务调用
type StoreLoad interface {
	// Refresh 普通全量：缓存 Ready 则跳过
	Refresh(ctx context.Context) error
	// RefreshForce 强制全量：回源覆盖并 reconcile 孤儿
	RefreshForce(ctx context.Context) error
	// RefreshScript 普通单脚本：该 id 已在缓存则跳过
	RefreshScript(ctx context.Context, id string) error
	// RefreshScriptForce 强制单脚本：回源覆盖；数据源 NotFound 则删除缓存中该 id
	RefreshScriptForce(ctx context.Context, id string) error
}

// StoreQuery 快照只读视图；由 golua 实现，供业务按需直读（Engine 执行路径不经过本接口）
type StoreQuery interface {
	// Get 按 ID 读缓存；miss 返回 ErrNotFound，不回源
	Get(ctx context.Context, id string) (*Script, error)
}

// ScriptInvalidator 本地编译缓存与池世代失效；由 Engine 实现，StoreLoad 在写成功后回调
type ScriptInvalidator interface {
	// InvalidateScripts 丢弃指定脚本的编译缓存并 bump 池世代、清空空闲旧 State
	InvalidateScripts(ids ...string)
	// InvalidateAllScripts 丢弃全部编译缓存并 bump 池世代
	InvalidateAllScripts()
	// ObserveGeneration 对齐共享代际水位（Redis）；本机 Refresh 后回填，避免多余全量失效
	ObserveGeneration(gen uint64)
}
