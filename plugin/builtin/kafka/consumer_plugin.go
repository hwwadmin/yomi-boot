package kafkaplugin

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/common/kafka"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/plugin"
)

const ConsumerName = "kafka-consumer"

// Consumer Kafka 消费者插件；自包含消费者管理器
type Consumer struct {
	plugin.Base

	consumer kafka.ConsumerService
}

var _ plugin.Plugin = (*Consumer)(nil)

// NewKafkaConsumer 创建 Kafka 消费者生命周期插件
func NewKafkaConsumer() *Consumer {
	return &Consumer{Base: plugin.NewBase(ConsumerName)}
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
func (p *Consumer) Init(ctx context.Context, slot *plugin.PluginSlot) error {
	bootCfg := config.MustSection[BootConfig](slot.Config, SectionKey)
	if !bootCfg.ConsumerEnabledOrDefault() {
		return plugin.ErrDisabled
	}
	consumer, err := kafka.NewConsumerManager(ctx, bootCfg.ToConsumerManagerConfig())
	if err != nil {
		return err
	}
	p.consumer = consumer
	return nil
}

// Close 停止全部 Kafka 消费者
func (p *Consumer) Close(context.Context) error {
	if p.consumer == nil {
		return nil
	}
	err := p.consumer.Close()
	p.consumer = nil
	return err
}

// Service 返回 Kafka 消费者管理器；未启用时返回 nil
func (p *Consumer) Service() kafka.ConsumerService {
	if p == nil {
		return nil
	}
	return p.consumer
}
