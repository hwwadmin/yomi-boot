// Package loggerplugin 日志插件
//
// @author zwei
package loggerplugin

import (
	"gitee.com/hq_project/yomi-boot/component/common/logger"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/plugin"
	appctxplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/appctx"
)

const Name = "logger"

// Logger 日志插件；日志枢纽为进程级单例，无需对外暴露组件
type Logger struct {
	plugin.PluginState

	closeFn func()
}

var _ plugin.Plugin = (*Logger)(nil)

// NewLogger 创建日志生命周期插件
func NewLogger() *Logger {
	return &Logger{PluginState: plugin.NewPluginState(Name)}
}

// Init 读取 log 配置并初始化日志枢纽
func (p *Logger) Init(slot *plugin.PluginSlot) {
	app := appctxplugin.From(slot).AppCtx()
	if app == nil {
		panic(errors.New("logger: 需要 app 插件"))
	}
	bootCfg := config.MustSection[BootConfig](slot.Config, SectionKey)
	p.closeFn = logger.MustInit(app.Name(), app.Env(), bootCfg.ToConfig())
	p.SetStarted()
}

// Close 关闭日志资源（刷缓冲、关文件）
func (p *Logger) Close() {
	if p.closeFn == nil {
		return
	}
	p.closeFn()
	p.closeFn = nil
}
