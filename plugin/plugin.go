// Package plugin 提供插件生命周期、统一插件插槽与进程阶段 hook。
//
// 插件实现 Plugin 接口，自包含其创建的资源；需要参与进程阶段的插件按需实现
// ReadyHook / DrainHook，由 lifecycle 在对应阶段统一调用。
//
// @author zwei
package plugin

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// ErrDisabled 表示插件未启用或依赖未满足；Init 返回该值时插件被标记为跳过，
// 不参与 Ready / Drain hook，也不会被 Close。
var ErrDisabled = errors.NewNoStack("plugin: disabled")

// Plugin 是插件生命周期接口；插件自包含其创建的资源。
type Plugin interface {
	// Name 返回插件名称，同一插件链内唯一。
	Name() string
	// Init 初始化插件；失败时返回 error，ctx 取消时返回 ctx.Err()。
	// 失败前须自行清理本次申请的资源；未启用时返回 ErrDisabled。
	Init(ctx context.Context, slot *PluginSlot) error
	// Close 释放插件资源；须尊重 ctx，可重复调用。
	Close(ctx context.Context) error
}

// ReadyHook 在 HTTP 监听成功、服务宣告就绪前执行；失败会导致启动失败。
type ReadyHook interface {
	OnReady(ctx context.Context) error
}

// DrainHook 在 HTTP 停止接收流量前执行；逆序调用，失败只记录日志。
type DrainHook interface {
	OnDraining(ctx context.Context) error
}

// Base 提供插件名称，供插件嵌入复用。
type Base struct {
	name string
}

// NewBase 创建插件名称基类。
func NewBase(name string) Base {
	return Base{name: name}
}

// Name 返回插件名称。
func (b *Base) Name() string {
	if b == nil {
		return ""
	}
	return b.name
}
