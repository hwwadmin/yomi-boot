// Package consulplugin Consul 插件
//
// @author zwei
package consulplugin

import (
	"context"

	"gitee.com/hq_project/yomi-boot/boot/plugin"
	"gitee.com/hq_project/yomi-boot/cloud/consul"
	"gitee.com/hq_project/yomi-boot/core/config"
	"gitee.com/hq_project/yomi-boot/core/errors"
)

// consulPlugin Consul 插件：Init 只创建客户端，注册/注销由进程阶段钩子完成
type consulPlugin struct {
	plugin.PluginState
	client    *consul.Client
	cfg       consul.Config
	appName   string
	serviceID string
}

const consulName = "consul"

var (
	_ plugin.ReadyHook = (*consulPlugin)(nil)
	_ plugin.DrainHook = (*consulPlugin)(nil)
)

// NewConsul 创建 Consul 生命周期插件
func NewConsul() plugin.Plugin {
	return &consulPlugin{PluginState: plugin.NewPluginState(consulName)}
}

// Init 初始化 Consul 客户端；服务注册推迟到 Ready 阶段
func (p *consulPlugin) Init(host *plugin.Host) {
	cfg := config.MustSection[consul.Config](host.Doc, consul.SectionKey)
	if !cfg.EnabledOrDefault() {
		return
	}
	if host.AppCtx == nil {
		panic(errors.New("consul: 需要 AppCtx"))
	}
	p.client = consul.MustNewClient(*cfg)
	p.cfg = *cfg
	p.appName = host.AppCtx.Name()
	host.ConsulClient = p.client
	p.SetStarted()
}

// OnReady 在 HTTP 监听成功后注册服务
func (p *consulPlugin) OnReady(_ context.Context) error {
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
func (p *consulPlugin) OnDraining(_ context.Context) error {
	if p.client == nil || p.serviceID == "" {
		return nil
	}
	return p.client.DeregisterService(p.serviceID)
}

// Close 无需额外释放；注销由 OnDraining 完成
func (p *consulPlugin) Close(_ *plugin.Host) {}
