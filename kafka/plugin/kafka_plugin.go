// Package kafkaplugin Kafka 插件
//
// @author zwei
package kafkaplugin

import (
	"yomi-boot/config"
	ytkafka "yomi-boot/kafka"
	"yomi-boot/plugin"
)

// kafkaPlugin Kafka 插件
type kafkaPlugin struct {
	plugin.PluginState
	components *ytkafka.Components
}

const kafkaName = "kafka"

// NewKafka 创建 Kafka 生命周期插件
func NewKafka() plugin.Plugin {
	return &kafkaPlugin{PluginState: plugin.NewPluginState(kafkaName)}
}

// Init 初始化 Kafka 组件并挂到 Host
func (p *kafkaPlugin) Init(host *plugin.Host) {
	cfg := config.MustSection[ytkafka.Config](host.Doc, ytkafka.SectionKey)
	if !cfg.EnabledOrDefault() {
		return
	}
	components := ytkafka.MustNewComponents(*cfg)
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
