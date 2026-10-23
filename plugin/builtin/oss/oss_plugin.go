// 对象存储生命周期插件。

package ossplugin

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/common/oss"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/plugin"
)

const Name = "oss"

// OSS 对象存储插件；自包含存储客户端
type OSS struct {
	plugin.Base

	storage oss.Storage
}

var _ plugin.Plugin = (*OSS)(nil)

// NewOSS 创建对象存储生命周期插件
func NewOSS() *OSS {
	return &OSS{Base: plugin.NewBase(Name)}
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
func (p *OSS) Init(_ context.Context, slot *plugin.PluginSlot) error {
	bootCfg := config.MustSection[BootConfig](slot.Config, SectionKey)
	if !bootCfg.EnabledOrDefault() {
		return plugin.ErrDisabled
	}
	storage, err := oss.New(bootCfg.ToConfig())
	if err != nil {
		return err
	}
	p.storage = storage
	return nil
}

// Close 对象存储无连接资源需要释放
func (p *OSS) Close(context.Context) error { return nil }

// Storage 返回对象存储；未启用时返回 nil
func (p *OSS) Storage() oss.Storage {
	if p == nil {
		return nil
	}
	return p.storage
}
