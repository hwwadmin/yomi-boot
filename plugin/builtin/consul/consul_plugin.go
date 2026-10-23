// Package consulplugin Consul 插件
//
// @author zwei
package consulplugin

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/cloud/consul"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/plugin"
	appctxplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/appctx"
)

const Name = "consul"

// Consul Consul 插件；自包含客户端与注册信息
type Consul struct {
	plugin.PluginState

	client    *consul.Client
	cfg       consul.Config
	appName   string
	serviceID string
}

var (
	_ plugin.Plugin    = (*Consul)(nil)
	_ plugin.ReadyHook = (*Consul)(nil)
	_ plugin.DrainHook = (*Consul)(nil)
)

// NewConsul 创建 Consul 生命周期插件
func NewConsul() *Consul {
	return &Consul{PluginState: plugin.NewPluginState(Name)}
}

// From 从插件插槽中取出 Consul 插件；未注册时返回 nil
func From(slot *plugin.PluginSlot) *Consul {
	if slot == nil {
		return nil
	}
	p, _ := slot.Plugin(Name)
	consulPlugin, _ := p.(*Consul)
	return consulPlugin
}

// Init 初始化 Consul 客户端；服务注册推迟到 Ready 阶段
func (p *Consul) Init(slot *plugin.PluginSlot) {
	bootCfg := config.MustSection[BootConfig](slot.Config, SectionKey)
	if !bootCfg.EnabledOrDefault() {
		return
	}
	app := appctxplugin.From(slot).AppCtx()
	if app == nil {
		panic(errors.New("consul: 需要 app 插件"))
	}
	p.cfg = bootCfg.ToConfig()
	p.client = consul.MustNewClient(p.cfg)
	p.appName = app.Name()
	p.SetStarted()
}

// OnReady 在 HTTP 监听成功后注册服务
func (p *Consul) OnReady(_ context.Context) error {
	if p.client == nil {
		return nil
	}
	serviceID, err := p.client.RegisterService(p.cfg, p.appName)
	if err != nil {
		return err
	}
	p.serviceID = serviceID
	return nil
}

// OnDraining 在 HTTP 停止接收流量前注销服务
func (p *Consul) OnDraining(_ context.Context) error {
	if p.client == nil || p.serviceID == "" {
		return nil
	}
	return p.client.DeregisterService(p.serviceID)
}

// Close 无需额外释放；注销由 OnDraining 完成
func (p *Consul) Close() {}

// Client 返回 Consul 客户端；未启用时返回 nil
func (p *Consul) Client() *consul.Client {
	if p == nil {
		return nil
	}
	return p.client
}
