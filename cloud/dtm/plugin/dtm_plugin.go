// Package dtmplugin DTM 插件
//
// @author zwei
package dtmplugin

import (
	"gitee.com/hq_project/yomi-boot/boot/config"
	"gitee.com/hq_project/yomi-boot/boot/plugin"
	"gitee.com/hq_project/yomi-boot/cloud/dtm"
)

// dtmPlugin DTM 插件
type dtmPlugin struct {
	plugin.PluginState
}

const dtmName = "dtm"

// NewDtm 创建 DTM 生命周期插件（写入 Host.DtmServer；enabled 时探活）
func NewDtm() plugin.Plugin {
	return &dtmPlugin{PluginState: plugin.NewPluginState(dtmName)}
}

// Init 初始化 DTM 服务并挂到 Host
func (p *dtmPlugin) Init(host *plugin.Host) {
	cfg := config.MustSection[dtm.Config](host.Doc, dtm.SectionKey)
	host.DtmServer = cfg.ResolveServer()
	if !cfg.EnabledOrDefault() {
		return
	}
	dtm.ProbeServer(host.DtmServer)
	p.SetStarted()
}

// Close 关闭 DTM 资源
func (p *dtmPlugin) Close(_ *plugin.Host) {}
