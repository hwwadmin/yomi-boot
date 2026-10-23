// Package appctxplugin 应用上下文插件
//
// @author zwei
package appctxplugin

import (
	"gitee.com/hq_project/yomi-boot/component/core/appctx"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/plugin"
)

const Name = "app"

// AppCtx 应用上下文插件；自包含 AppCtx 组件，始终启动
type AppCtx struct {
	plugin.PluginState

	appCtx appctx.AppCtx
}

var _ plugin.Plugin = (*AppCtx)(nil)

// NewAppCtx 创建应用上下文生命周期插件
func NewAppCtx() *AppCtx {
	return &AppCtx{PluginState: plugin.NewPluginState(Name)}
}

// From 从插件插槽中取出应用上下文插件；未注册时返回 nil
func From(slot *plugin.PluginSlot) *AppCtx {
	if slot == nil {
		return nil
	}
	p, _ := slot.Plugin(Name)
	appCtxPlugin, _ := p.(*AppCtx)
	return appCtxPlugin
}

// Init 初始化 AppCtx 并锁定时区
func (p *AppCtx) Init(slot *plugin.PluginSlot) {
	bootCfg := config.MustSection[BootConfig](slot.Config, SectionKey)
	cfg := bootCfg.ToConfig()
	if err := appctx.LockTimezone(cfg.Timezone); err != nil {
		panic(errors.Wrap(err, "appctx: 锁定时区失败"))
	}
	p.appCtx = appctx.NewAppCtx(cfg.Name, cfg.Env, cfg.Version)
	p.SetStarted()
}

// Close 应用上下文无资源需要释放
func (p *AppCtx) Close() {}

// AppCtx 返回应用上下文；插件未初始化时返回 nil
func (p *AppCtx) AppCtx() appctx.AppCtx {
	if p == nil {
		return nil
	}
	return p.appCtx
}
