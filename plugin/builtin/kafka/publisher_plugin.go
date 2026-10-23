package kafkaplugin

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/common/kafka"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/plugin"
)

const PublisherName = "kafka-publisher"

// Publisher Kafka 生产者插件；自包含生产者
type Publisher struct {
	plugin.Base

	publisher kafka.PublisherService
}

var _ plugin.Plugin = (*Publisher)(nil)

// NewKafkaPublisher 创建 Kafka 生产者生命周期插件
func NewKafkaPublisher() *Publisher {
	return &Publisher{Base: plugin.NewBase(PublisherName)}
}

// FromPublisher 从插件插槽中取出 Kafka 生产者插件；未注册时返回 nil
func FromPublisher(slot *plugin.PluginSlot) *Publisher {
	if slot == nil {
		return nil
	}
	p, _ := slot.Plugin(PublisherName)
	publisherPlugin, _ := p.(*Publisher)
	return publisherPlugin
}

// Init 初始化 Kafka 生产者
func (p *Publisher) Init(ctx context.Context, slot *plugin.PluginSlot) error {
	bootCfg := config.MustSection[BootConfig](slot.Config, SectionKey)
	if !bootCfg.PublisherEnabledOrDefault() {
		return plugin.ErrDisabled
	}
	publisher, err := kafka.NewPublisher(ctx, bootCfg.ToPublisherConfig())
	if err != nil {
		return err
	}
	p.publisher = publisher
	return nil
}

// Close 关闭 Kafka 生产者
func (p *Publisher) Close(context.Context) error {
	if p.publisher == nil {
		return nil
	}
	err := p.publisher.Close()
	p.publisher = nil
	return err
}

// Service 返回 Kafka 生产者；未启用时返回 nil
func (p *Publisher) Service() kafka.PublisherService {
	if p == nil {
		return nil
	}
	return p.publisher
}
