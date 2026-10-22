package kafkaplugin

import (
	"gitee.com/hq_project/yomi-boot/component/common/kafka"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/plugin"
)

// kafkaPublisherPlugin Kafka 生产者插件
type kafkaPublisherPlugin struct {
	plugin.PluginState
	publisher kafka.PublisherService
}

const publisherName = "kafka-publisher"

// NewKafkaPublisher 创建 Kafka 生产者生命周期插件
func NewKafkaPublisher() plugin.Plugin {
	return &kafkaPublisherPlugin{PluginState: plugin.NewPluginState(publisherName)}
}

// Init 初始化 Kafka 生产者并挂到 Host
func (p *kafkaPublisherPlugin) Init(host *plugin.Host) {
	bootCfg := config.MustSection[BootConfig](host.Config, SectionKey)
	if !bootCfg.PublisherEnabledOrDefault() {
		return
	}
	p.publisher = kafka.MustNewPublisher(bootCfg.ToPublisherConfig())
	host.KafkaPublisher = p.publisher
	p.SetStarted()
}

// Close 关闭 Kafka 生产者
func (p *kafkaPublisherPlugin) Close(_ *plugin.Host) {
	if p.publisher == nil {
		return
	}
	_ = p.publisher.Close()
	p.publisher = nil
}
