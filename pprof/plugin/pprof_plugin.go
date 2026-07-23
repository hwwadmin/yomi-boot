// Package pprofplugin pprof 性能剖析插件
//
// @author zwei
package pprofplugin

import (
	"yomi-boot/config"
	"yomi-boot/plugin"
	bootpprof "yomi-boot/pprof"
)

// pprofPlugin pprof 性能剖析插件
type pprofPlugin struct {
	plugin.PluginState
}

const pprofName = "pprof"

// NewPprof 创建 pprof 生命周期插件
func NewPprof() plugin.Plugin {
	return &pprofPlugin{PluginState: plugin.NewPluginState(pprofName)}
}

// Init 初始化 pprof 性能剖析服务
func (p *pprofPlugin) Init(host *plugin.Host) {
	cfg := config.MustSection[bootpprof.Config](host.Doc, bootpprof.SectionKey)
	bootpprof.MustStart(*cfg)
	p.SetStarted()
}

// Close 关闭 pprof 资源
func (p *pprofPlugin) Close(_ *plugin.Host) {}
