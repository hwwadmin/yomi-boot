// 插件扩展选项。

package plugin

import "gitee.com/hq_project/yomi-boot/component/core/config"

// PluginOptions 插件扩展选项；自定义插件与自定义配置段都属于插件链的一环，
// 各能力是否启动由插件根据配置自行判断。
type PluginOptions struct {
	// customPlugins 服务自定义插件
	customPlugins []Plugin
	// customConfigs 业务等自定义配置段，在默认基础设施段之后注册
	customConfigs []config.Section
}

// AddCustomPlugin 注册服务自定义插件（Init 挂在内置插件链末尾，晚于 event；可依赖 KafkaConsumer / Event.Receiver 等）
func (opts *PluginOptions) AddCustomPlugin(customPlugin Plugin) *PluginOptions {
	opts.customPlugins = append(opts.customPlugins, customPlugin)
	return opts
}

// AddConfigSection 注册自定义配置段（须在默认内置配置段之后追加）
func (opts *PluginOptions) AddConfigSection(section config.Section) *PluginOptions {
	opts.customConfigs = append(opts.customConfigs, section)
	return opts
}

// CustomPlugins 返回已注册的服务自定义插件
func (opts *PluginOptions) CustomPlugins() []Plugin {
	return opts.customPlugins
}

// CustomConfigs 返回已注册的自定义配置段
func (opts *PluginOptions) CustomConfigs() []config.Section {
	return opts.customConfigs
}
