// 对象存储生命周期插件。

package ossplugin

import (
	"gitee.com/hq_project/yomi-boot/component/common/oss"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/plugin"
)

const Name = "oss"

// OSS 对象存储插件；自包含存储客户端
type OSS struct {
	plugin.PluginState

	storage oss.Storage
}

var _ plugin.Plugin = (*OSS)(nil)

// NewOSS 创建对象存储生命周期插件
func NewOSS() *OSS {
	return &OSS{PluginState: plugin.NewPluginState(Name)}
}

// From 从插件插槽中取出对象存储插件；未注册时返回 nil
func From(slot *plugin.PluginSlot) *OSS {
	if slot == nil {
		return nil
	}
	p, _ := slot.Plugin(Name)
	ossPlugin, _ := p.(*OSS)
	return ossPlugin
}

// Init 初始化对象存储
func (p *OSS) Init(slot *plugin.PluginSlot) {
	bootCfg := config.MustSection[BootConfig](slot.Config, SectionKey)
	if !bootCfg.EnabledOrDefault() {
		return
	}
	p.storage = oss.MustNew(bootCfg.ToConfig())
	p.SetStarted()
}

// Close 对象存储无连接资源需要释放
func (p *OSS) Close() {}

// Storage 返回对象存储；未启用时返回 nil
func (p *OSS) Storage() oss.Storage {
	if p == nil {
		return nil
	}
	return p.storage
}
