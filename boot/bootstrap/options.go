// 基础设施初始化扩展选项。

package bootstrap

import (
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/plugin"
)

// InitOptions 初始化扩展选项；各能力是否启动由插件根据配置自行判断
type InitOptions struct {
	// customPlugins 服务自定义插件
	customPlugins []plugin.Plugin
	// configSections 业务等自定义配置段，在默认基础设施段之后注册
	configSections []config.Section
}

// AddCustomPlugin 注册服务自定义插件（Init 挂在内置插件链末尾，晚于 event；可依赖 KafkaConsumer / Event.Receiver 等）
func (o *InitOptions) AddCustomPlugin(p plugin.Plugin) *InitOptions {
	o.customPlugins = append(o.customPlugins, p)
	return o
}

// AddConfigSection 注册自定义配置段（须在默认内置配置段之后追加）
func (o *InitOptions) AddConfigSection(s config.Section) *InitOptions {
	o.configSections = append(o.configSections, s)
	return o
}
