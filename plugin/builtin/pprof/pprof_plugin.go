// Package pprofplugin pprof 性能剖析插件
//
// @author zwei
package pprofplugin

import (
	"context"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/logger"
	componentpprof "gitee.com/hq_project/yomi-boot/component/common/pprof"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/plugin"
)

const (
	Name        = "pprof"
	stopTimeout = 3 * time.Second
)

// Pprof pprof 性能剖析插件；自包含剖析服务
type Pprof struct {
	plugin.PluginState

	server *componentpprof.Server
}

var _ plugin.Plugin = (*Pprof)(nil)

// NewPprof 创建 pprof 生命周期插件
func NewPprof() *Pprof {
	return &Pprof{PluginState: plugin.NewPluginState(Name)}
}

// Init 初始化 pprof 性能剖析服务；未启用时不标记 Started
func (p *Pprof) Init(slot *plugin.PluginSlot) {
	bootCfg := config.MustSection[BootConfig](slot.Config, SectionKey)
	if !bootCfg.EnabledOrDefault() {
		return
	}
	p.server = componentpprof.MustStart(bootCfg.ToConfig())
	p.SetStarted()
}

// Close 优雅停止 pprof 服务
func (p *Pprof) Close() {
	if p.server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
	defer cancel()
	if err := p.server.Stop(ctx); err != nil {
		logger.WarnWithoutCtx("pprof 服务停止失败", "error", err)
	}
	p.server = nil
}
