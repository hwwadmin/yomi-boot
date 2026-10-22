// Package appctxplugin 应用上下文插件
//
// @author zwei
package appctxplugin

import (
	"gitee.com/hq_project/yomi-boot/component/common/appctx"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/plugin"
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

// Init 初始化 AppCtx、锁定时区并挂到 Host
func (p *appCtxPlugin) Init(host *plugin.Host) {
	bootCfg := config.MustSection[BootConfig](host.Doc, SectionKey)
	cfg := bootCfg.ToConfig()
	if err := appctx.LockTimezone(cfg.Timezone); err != nil {
		panic(errors.Wrap(err, "appctx: 锁定时区失败"))
	}
	host.AppCtx = appctx.NewAppCtx(cfg.Name, cfg.Env, cfg.Version)
	p.SetStarted()
}

// Close 关闭应用上下文资源
func (p *appCtxPlugin) Close(_ *plugin.Host) {}
