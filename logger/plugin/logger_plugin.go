// Package loggerplugin 日志插件
//
// @author zwei
package loggerplugin

import (
	"yomi-boot/config"
	"yomi-boot/errors"
	"yomi-boot/logger"
	"yomi-boot/plugin"
)

// loggerPlugin 日志插件
type loggerPlugin struct {
	plugin.PluginState
	closeFn func()
}

const loggerName = "logger"

// NewLogger 创建日志生命周期插件
func NewLogger() plugin.Plugin {
	return &loggerPlugin{PluginState: plugin.NewPluginState(loggerName)}
}

// Init 读取 log 配置并初始化日志枢纽
func (p *loggerPlugin) Init(host *plugin.Host) {
	if host.AppCtx == nil {
		panic(errors.New("logger: 需要 AppCtx"))
	}
	logCfg := config.MustSection[logger.Config](host.Doc, logger.SectionKey)
	p.closeFn = logger.MustInit(host.AppCtx.Name(), host.AppCtx.Env(), *logCfg)
	p.SetStarted()
}

// Close 关闭日志资源（刷缓冲、关文件）
func (p *loggerPlugin) Close(_ *plugin.Host) {
	if p.closeFn != nil {
		p.closeFn()
	}
}
