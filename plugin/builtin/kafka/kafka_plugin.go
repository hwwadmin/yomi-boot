// Package kafkaplugin Kafka 插件
//
// @author zwei
package kafkaplugin

import (
	yomikafka "gitee.com/hq_project/yomi-boot/component/common/kafka"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/plugin"
)

// kafkaPlugin Kafka 插件
type kafkaPlugin struct {
	plugin.PluginState
	components *yomikafka.Components
}

const kafkaName = "kafka"

// NewKafka 创建 Kafka 生命周期插件
func NewKafka() plugin.Plugin {
	return &kafkaPlugin{PluginState: plugin.NewPluginState(kafkaName)}
}

// Init 初始化 Kafka 组件并挂到 Host
func (p *kafkaPlugin) Init(host *plugin.Host) {
	bootCfg := config.MustSection[BootConfig](host.Config, SectionKey)
	if !bootCfg.EnabledOrDefault() {
		return
	}
	components := yomikafka.MustNewComponents(bootCfg.ToConfig())
	p.components = components
	host.KafkaComponents = components
	host.KafkaPublisher = components.Publisher
	host.KafkaConsumer = components.Consumer
	p.SetStarted()
}

// Close 关闭 Kafka 资源
func (p *kafkaPlugin) Close(_ *plugin.Host) {
	if p.components == nil {
		return
	}
	_ = p.components.Close()
}
