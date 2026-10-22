// Package dtmplugin DTM 插件
//
// @author zwei
package dtmplugin

import (
	"gitee.com/hq_project/yomi-boot/component/cloud/dtm"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/plugin"
)

// dtmPlugin DTM 插件
type dtmPlugin struct {
	plugin.PluginState
}

const dtmName = "dtm"

// NewDtm 创建 DTM 生命周期插件（构造 Host.DtmClient；enabled 时探活）
func NewDtm() plugin.Plugin {
	return &dtmPlugin{PluginState: plugin.NewPluginState(dtmName)}
}

// Init 初始化 DTM 客户端并挂到 Host
func (p *dtmPlugin) Init(host *plugin.Host) {
	bootCfg := config.MustSection[BootConfig](host.Config, SectionKey)
	cfg := bootCfg.ToConfig()
	enabled := bootCfg.EnabledOrDefault()
	server := ""
	if enabled {
		server = cfg.ResolveServer()
	}
	host.DtmClient = dtm.NewClient(host.ConsulClient, server, dtm.WithTimeout(cfg.TimeoutOrDefault()))
	if !enabled {
		return
	}
	dtm.ProbeServer(server)
	p.SetStarted()
}

// Close 关闭 DTM 资源
func (p *dtmPlugin) Close(_ *plugin.Host) {}
