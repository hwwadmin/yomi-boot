// Package idempotencyplugin 幂等 Processing 超时清理插件
//
// @author zwei
package idempotencyplugin

import (
	"context"

	"gitee.com/hq_project/yomi-boot/boot/plugin"
	"gitee.com/hq_project/yomi-boot/component/idempotency"
	"gitee.com/hq_project/yomi-boot/core/config"
	"gitee.com/hq_project/yomi-boot/core/errors"
)

// idempotencyStaleCleanerPlugin 幂等 Processing 超时清理插件
type idempotencyStaleCleanerPlugin struct {
	plugin.PluginState
	cleaner *idempotency.StaleCleaner
}

const idempotencyName = "idempotency"

// NewIdempotencyStaleCleaner 创建幂等 Processing 超时清理生命周期插件
func NewIdempotencyStaleCleaner() plugin.Plugin {
	return &idempotencyStaleCleanerPlugin{PluginState: plugin.NewPluginState(idempotencyName)}
}

// Init 初始化幂等超时清理器并挂到 Host
func (p *idempotencyStaleCleanerPlugin) Init(host *plugin.Host) {
	cfg := config.MustSection[idempotency.Config](host.Doc, idempotency.SectionKey)
	if !cfg.StaleCleanerEnabledOrDefault() {
		return
	}
	if host.AppCtx == nil {
		panic(errors.New("idempotency: 需要 AppCtx"))
	}
	p.cleaner = idempotency.MustStartStaleCleanerFromConfig(context.Background(), idempotency.StaleCleanerDeps{
		Repo:            idempotency.NewRepository(host.DB),
		DistLockFactory: host.DistLockFactory,
		AppName:         host.AppCtx.Name(),
		Config:          *cfg,
	})
	if p.cleaner != nil {
		p.SetStarted()
	}
}

// Close 关闭幂等超时清理资源
func (p *idempotencyStaleCleanerPlugin) Close(_ *plugin.Host) {
	if p.cleaner == nil {
		return
	}
	_ = p.cleaner.Close()
}
