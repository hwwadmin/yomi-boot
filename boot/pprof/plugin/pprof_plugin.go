// Package pprofplugin pprof 性能剖析插件
//
// @author zwei
package pprofplugin

import (
	"context"
	"time"

	"gitee.com/hq_project/yomi-boot/boot/config"
	"gitee.com/hq_project/yomi-boot/boot/logger"
	"gitee.com/hq_project/yomi-boot/boot/plugin"
	bootpprof "gitee.com/hq_project/yomi-boot/boot/pprof"
)

const (
	pprofName   = "pprof"
	stopTimeout = 3 * time.Second
)

// pprofPlugin pprof 性能剖析插件
type pprofPlugin struct {
	plugin.PluginState
	server *bootpprof.Server
}

// NewPprof 创建 pprof 生命周期插件
func NewPprof() plugin.Plugin {
	return &pprofPlugin{PluginState: plugin.NewPluginState(pprofName)}
}

// Init 初始化 pprof 性能剖析服务；未启用时不标记 Started
func (p *pprofPlugin) Init(host *plugin.Host) {
	cfg := config.MustSection[bootpprof.Config](host.Doc, bootpprof.SectionKey)
	p.server = bootpprof.MustStart(*cfg)
	if p.server == nil {
		return
	}
	p.SetStarted()
}

// Close 优雅停止 pprof 服务
func (p *pprofPlugin) Close(_ *plugin.Host) {
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
