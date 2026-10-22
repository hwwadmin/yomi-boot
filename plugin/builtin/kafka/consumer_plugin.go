package kafkaplugin

import (
	"gitee.com/hq_project/yomi-boot/component/common/kafka"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/plugin"
)

// kafkaConsumerPlugin Kafka 消费者插件
type kafkaConsumerPlugin struct {
	plugin.PluginState
	consumer kafka.ConsumerService
}

const consumerName = "kafka-consumer"

// NewKafkaConsumer 创建 Kafka 消费者生命周期插件
func NewKafkaConsumer() plugin.Plugin {
	return &kafkaConsumerPlugin{PluginState: plugin.NewPluginState(consumerName)}
}

// Init 初始化 Kafka 消费者管理器并挂到 Host
func (p *kafkaConsumerPlugin) Init(host *plugin.Host) {
	bootCfg := config.MustSection[BootConfig](host.Config, SectionKey)
	if !bootCfg.ConsumerEnabledOrDefault() {
		return
	}
	p.consumer = kafka.MustNewConsumerManager(bootCfg.ToConsumerManagerConfig())
	host.KafkaConsumer = p.consumer
	p.SetStarted()
}

// Close 停止全部 Kafka 消费者
func (p *kafkaConsumerPlugin) Close(_ *plugin.Host) {
	if p.consumer == nil {
		return
	}
	_ = p.consumer.Close()
	p.consumer = nil
}
