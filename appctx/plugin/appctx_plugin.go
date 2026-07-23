// Package appctxplugin 应用上下文插件
//
// @author zwei
package appctxplugin

import (
	"yomi-boot/appctx"
	"yomi-boot/config"
	"yomi-boot/plugin"
)

// appCtxPlugin 应用上下文插件
type appCtxPlugin struct {
	plugin.PluginState
}

const appCtxName = "app"

// NewAppCtx 创建应用上下文生命周期插件（始终启动，供后续插件读取 AppCtx）
func NewAppCtx() plugin.Plugin {
	return &appCtxPlugin{PluginState: plugin.NewPluginState(appCtxName)}
}

// Init 初始化 AppCtx 并挂到 Host
func (p *appCtxPlugin) Init(host *plugin.Host) {
	cfg := config.MustSection[appctx.Config](host.Doc, appctx.SectionKey)
	host.AppCtx = appctx.NewAppCtx(cfg.Name, cfg.Env, cfg.Version)
	p.SetStarted()
}

// Close 关闭应用上下文资源
func (p *appCtxPlugin) Close(_ *plugin.Host) {}
