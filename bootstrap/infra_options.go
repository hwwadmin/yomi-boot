// Package bootstrap 基础设施选项
//
// @author zwei
package bootstrap

import (
	"yt-boot/config"
	"yt-boot/plugin"
)

// InfraOptions 基础设施扩展选项；各组件是否启动由插件根据配置自行判断
type InfraOptions struct {
	// customPlugins 服务自定义插件
	customPlugins []plugin.Plugin
	// configSections 业务等自定义配置段，在默认基础设施段之后注册
	configSections []config.Section
}

// AddCustomPlugin 注册服务自定义插件（Init 挂在内置插件链末尾，晚于 event；可依赖 KafkaConsumer / Event.Receiver 等）
func (o *InfraOptions) AddCustomPlugin(p plugin.Plugin) *InfraOptions {
	o.customPlugins = append(o.customPlugins, p)
	return o
}

// AddConfigSection 注册自定义配置段（须在默认 boot 段之后追加）
func (o *InfraOptions) AddConfigSection(s config.Section) *InfraOptions {
	o.configSections = append(o.configSections, s)
	return o
}
