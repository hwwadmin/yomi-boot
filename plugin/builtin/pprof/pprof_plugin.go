// Package pprofplugin pprof 性能剖析插件
//
// @author zwei
package pprofplugin

import (
	"context"

	componentpprof "gitee.com/hq_project/yomi-boot/component/common/pprof"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/plugin"
)

const Name = "pprof"

// Pprof pprof 性能剖析插件；自包含剖析服务
type Pprof struct {
	plugin.Base

	server *componentpprof.Server
}

var _ plugin.Plugin = (*Pprof)(nil)

// NewPprof 创建 pprof 生命周期插件
func NewPprof() *Pprof {
	return &Pprof{Base: plugin.NewBase(Name)}
}

// Init 初始化 pprof 性能剖析服务
func (p *Pprof) Init(_ context.Context, slot *plugin.PluginSlot) error {
	bootCfg := config.MustSection[BootConfig](slot.Config, SectionKey)
	if !bootCfg.EnabledOrDefault() {
		return plugin.ErrDisabled
	}
	server, err := componentpprof.Start(bootCfg.ToConfig())
	if err != nil {
		return err
	}
	p.server = server
	return nil
}

// Close 优雅停止 pprof 服务
func (p *Pprof) Close(ctx context.Context) error {
	if p.server == nil {
		return nil
	}
	err := p.server.Stop(ctx)
	p.server = nil
	return err
}
