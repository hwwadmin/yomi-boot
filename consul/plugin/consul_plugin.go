// Package consulplugin Consul 插件
//
// @author zwei
package consulplugin

import (
	"gitee.com/hq_project/yomi-boot/config"
	"gitee.com/hq_project/yomi-boot/consul"
	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/plugin"
)

// consulPlugin Consul 插件
type consulPlugin struct {
	plugin.PluginState
}

const consulName = "consul"

// NewConsul 创建 Consul 生命周期插件
func NewConsul() plugin.Plugin {
	return &consulPlugin{PluginState: plugin.NewPluginState(consulName)}
}

// Init 初始化 Consul 客户端并挂到 Host
func (p *consulPlugin) Init(host *plugin.Host) {
	cfg := config.MustSection[consul.Config](host.Doc, consul.SectionKey)
	if !cfg.EnabledOrDefault() {
		return
	}
	if host.AppCtx == nil {
		panic(errors.New("consul: 需要 AppCtx"))
	}
	client := consul.MustNewClient(*cfg)
	serviceID := consul.MustRegisterService(client, *cfg, host.AppCtx.Name())
	host.ConsulClient = client
	host.ConsulServiceID = serviceID
	p.SetStarted()
}

// Close 关闭 Consul 资源
func (p *consulPlugin) Close(host *plugin.Host) {
	if host.ConsulClient == nil || host.ConsulServiceID == "" {
		return
	}
	_ = host.ConsulClient.DeregisterService(host.ConsulServiceID)
}
