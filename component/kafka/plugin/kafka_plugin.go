// Package kafkaplugin Kafka 插件
//
// @author zwei
package kafkaplugin

import (
	"gitee.com/hq_project/yomi-boot/boot/plugin"
	yomikafka "gitee.com/hq_project/yomi-boot/component/kafka"
	"gitee.com/hq_project/yomi-boot/core/config"
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
	cfg := config.MustSection[yomikafka.Config](host.Doc, yomikafka.SectionKey)
	if !cfg.EnabledOrDefault() {
		return
	}
	components := yomikafka.MustNewComponents(*cfg)
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
