package kafkaplugin

import (
	"gitee.com/hq_project/yomi-boot/component/common/kafka"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/plugin"
)

const ConsumerName = "kafka-consumer"

// Consumer Kafka 消费者插件；自包含消费者管理器
type Consumer struct {
	plugin.PluginState

	consumer kafka.ConsumerService
}

var _ plugin.Plugin = (*Consumer)(nil)

// NewKafkaConsumer 创建 Kafka 消费者生命周期插件
func NewKafkaConsumer() *Consumer {
	return &Consumer{PluginState: plugin.NewPluginState(ConsumerName)}
}

// FromConsumer 从插件插槽中取出 Kafka 消费者插件；未注册时返回 nil
func FromConsumer(slot *plugin.PluginSlot) *Consumer {
	if slot == nil {
		return nil
	}
	p, _ := slot.Plugin(ConsumerName)
	consumerPlugin, _ := p.(*Consumer)
	return consumerPlugin
}

// Init 初始化 Kafka 消费者管理器
func (p *Consumer) Init(slot *plugin.PluginSlot) {
	bootCfg := config.MustSection[BootConfig](slot.Config, SectionKey)
	if !bootCfg.ConsumerEnabledOrDefault() {
		return
	}
	p.consumer = kafka.MustNewConsumerManager(bootCfg.ToConsumerManagerConfig())
	p.SetStarted()
}

// Close 停止全部 Kafka 消费者
func (p *Consumer) Close() {
	if p.consumer == nil {
		return
	}
	_ = p.consumer.Close()
	p.consumer = nil
}

// Service 返回 Kafka 消费者管理器；未启用时返回 nil
func (p *Consumer) Service() kafka.ConsumerService {
	if p == nil {
		return nil
	}
	return p.consumer
}
