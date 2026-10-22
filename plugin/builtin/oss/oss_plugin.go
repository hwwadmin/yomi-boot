// 对象存储生命周期插件。

package ossplugin

import (
	"gitee.com/hq_project/yomi-boot/component/common/oss"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/plugin"
)

// ossPlugin 对象存储插件
type ossPlugin struct {
	plugin.PluginState
}

const ossName = "oss"

// NewOSS 创建对象存储生命周期插件
func NewOSS() plugin.Plugin {
	return &ossPlugin{PluginState: plugin.NewPluginState(ossName)}
}

// Init 初始化对象存储并挂到 Host
func (p *ossPlugin) Init(host *plugin.Host) {
	bootCfg := config.MustSection[BootConfig](host.Doc, SectionKey)
	if !bootCfg.EnabledOrDefault() {
		return
	}
	host.OSS = oss.MustNew(bootCfg.ToConfig())
	p.SetStarted()
}

// Close 对象存储无连接资源需释放
func (p *ossPlugin) Close(_ *plugin.Host) {}
