package kafkaplugin

import (
	"gitee.com/hq_project/yomi-boot/component/common/kafka"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/plugin"
)

const PublisherName = "kafka-publisher"

// Publisher Kafka 生产者插件；自包含生产者
type Publisher struct {
	plugin.PluginState

	publisher kafka.PublisherService
}

var _ plugin.Plugin = (*Publisher)(nil)

// NewKafkaPublisher 创建 Kafka 生产者生命周期插件
func NewKafkaPublisher() *Publisher {
	return &Publisher{PluginState: plugin.NewPluginState(PublisherName)}
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
func (p *Publisher) Init(slot *plugin.PluginSlot) {
	bootCfg := config.MustSection[BootConfig](slot.Config, SectionKey)
	if !bootCfg.PublisherEnabledOrDefault() {
		return
	}
	p.publisher = kafka.MustNewPublisher(bootCfg.ToPublisherConfig())
	p.SetStarted()
}

// Close 关闭 Kafka 生产者
func (p *Publisher) Close() {
	if p.publisher == nil {
		return
	}
	_ = p.publisher.Close()
	p.publisher = nil
}

// Service 返回 Kafka 生产者；未启用时返回 nil
func (p *Publisher) Service() kafka.PublisherService {
	if p == nil {
		return nil
	}
	return p.publisher
}
