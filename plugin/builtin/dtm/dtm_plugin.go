// Package dtmplugin DTM 插件
//
// @author zwei
package dtmplugin

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/cloud/dtm"
	"gitee.com/hq_project/yomi-boot/component/common/db/tx"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/plugin"
	consulplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/consul"
	dbplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/db"
)

const Name = "dtm"

// Dtm DTM 插件；自包含 DTM 客户端与事务执行器
type Dtm struct {
	plugin.Base

	client *dtm.Client
	runner tx.Runner
}

var _ plugin.Plugin = (*Dtm)(nil)

// NewDtm 创建 DTM 生命周期插件；客户端始终构造，enabled 时探活并构造事务执行器
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

// Init 构造 DTM 客户端；enabled 时探活并在 db 可用时构造事务执行器
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

	dbPlugin := dbplugin.FromDB(slot)
	if dbPlugin != nil && dbPlugin.DB() != nil {
		p.runner = dtm.NewTxRunner(dbPlugin.DB(), dbPlugin.Type(), cfg.BarrierTableOrDefault())
	}
	return nil
}

// Close DTM 客户端与事务执行器无资源需要释放
func (p *Dtm) Close(context.Context) error { return nil }

// Client 返回 DTM 客户端；未初始化时返回 nil
func (p *Dtm) Client() *dtm.Client {
	if p == nil {
		return nil
	}
	return p.client
}

// Runner 返回 DTM 事务执行器；dtm 未启用或 db 插件未启用时返回 nil
func (p *Dtm) Runner() tx.Runner {
	if p == nil {
		return nil
	}
	return p.runner
}
