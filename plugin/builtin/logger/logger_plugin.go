// Package loggerplugin 日志插件
//
// @author zwei
package loggerplugin

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/logger"
	"gitee.com/hq_project/yomi-boot/plugin"
	appctxplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/appctx"
	maskingplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/masking"
)

const Name = "logger"

// Logger 日志插件；日志枢纽为进程级单例，无需对外暴露组件
type Logger struct {
	plugin.Base

	closeFn func() error
}

var _ plugin.Plugin = (*Logger)(nil)

// NewLogger 创建日志生命周期插件
func NewLogger() *Logger {
	return &Logger{Base: plugin.NewBase(Name)}
}

// Init 读取 log 配置并初始化日志枢纽；脱敏服务无条件注入，是否调用由组件按 mask_sensitive 判定
func (p *Logger) Init(_ context.Context, slot *plugin.PluginSlot) error {
	app := appctxplugin.From(slot).AppCtx()
	if app == nil {
		return errors.New("logger: 需要 app 插件")
	}
	bootCfg := config.MustSection[BootConfig](slot.Config, SectionKey)
	closeFn, err := logger.InitLogger(app.Name(), app.Env(), bootCfg.ToConfig(), maskingplugin.From(slot).Service())
	if err != nil {
		return err
	}
	p.closeFn = closeFn
	return nil
}

// Close 关闭日志资源（刷缓冲、关文件）
func (p *Logger) Close(context.Context) error {
	if p.closeFn == nil {
		return nil
	}
	err := p.closeFn()
	p.closeFn = nil
	return err
}
