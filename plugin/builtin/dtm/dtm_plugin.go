// Package dtmplugin DTM 插件
//
// @author zwei
package dtmplugin

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/cloud/dtm"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/plugin"
	consulplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/consul"
)

const Name = "dtm"

// Dtm DTM 插件；自包含 DTM 客户端
type Dtm struct {
	plugin.Base

	client *dtm.Client
}

var _ plugin.Plugin = (*Dtm)(nil)

// NewDtm 创建 DTM 生命周期插件；客户端始终构造，enabled 时探活
func NewDtm() *Dtm {
	return &Dtm{Base: plugin.NewBase(Name)}
}

// From 从插件插槽中取出 DTM 插件；未注册时返回 nil
func From(slot *plugin.PluginSlot) *Dtm {
	if slot == nil {
		return nil
	}
	p, _ := slot.Plugin(Name)
	dtmPlugin, _ := p.(*Dtm)
	return dtmPlugin
}

// Init 构造 DTM 客户端；enabled 时探活
func (p *Dtm) Init(_ context.Context, slot *plugin.PluginSlot) error {
	bootCfg := config.MustSection[BootConfig](slot.Config, SectionKey)
	cfg := bootCfg.ToConfig()
	enabled := bootCfg.EnabledOrDefault()
	server := ""
	if enabled {
		server = cfg.ResolveServer()
	}
	consul := consulplugin.From(slot)
	p.client = dtm.NewClient(consul.Client(), server, dtm.WithTimeout(cfg.TimeoutOrDefault()))
	if !enabled {
		return plugin.ErrDisabled
	}
	dtm.ProbeServer(server)
	return nil
}

// Close DTM 客户端无资源需要释放
func (p *Dtm) Close(context.Context) error { return nil }

// Client 返回 DTM 客户端；未初始化时返回 nil
func (p *Dtm) Client() *dtm.Client {
	if p == nil {
		return nil
	}
	return p.client
}
