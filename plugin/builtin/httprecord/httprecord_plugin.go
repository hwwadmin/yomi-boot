// Package httprecordplugin HTTP 访问记录插件
//
// @author zwei
package httprecordplugin

import (
	"context"
	"sync"

	"gitee.com/hq_project/yomi-boot/component/common/httprecord"
	yomikafka "gitee.com/hq_project/yomi-boot/component/common/kafka"
	"gitee.com/hq_project/yomi-boot/component/common/redisstream"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/plugin"
	kafkaplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/kafka"
	redisstreamplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/redisstream"
)

const Name = "httprecord"

// HttpRecord HTTP 访问记录插件；按 provider 自包含装配生产端与消费端。
// 消费处理函数由业务在 OnReady 之前通过 MustRegisterHandler 注入。
type HttpRecord struct {
	plugin.Base

	service   httprecord.HttpRecordService
	memorySvc *httprecord.MemoryHttpRecordService

	cfg        httprecord.Config // cfg 是本次启动的运行时配置
	provider   string            // provider 是解析后的实现提供商
	runtimeCtx context.Context   // runtimeCtx 是消费协程的运行期上下文

	kafkaConsumer  yomikafka.ConsumerService   // kafkaConsumer 是 kafka 消费端（启用消费时持有）
	streamConsumer redisstream.ConsumerService // streamConsumer 是 Redis Stream 消费端（启用消费时持有）
	consumerGroup  string                      // consumerGroup 是解析后的消费组标识
	consumerName   string                      // consumerName 是 Redis Stream 同组内唯一的消费者名

	handlerMu sync.Mutex               // handlerMu 保护 handler 的注册与读取
	handler   httprecord.RecordHandler // handler 是业务注入的消费处理函数
}

var (
	_ plugin.Plugin    = (*HttpRecord)(nil)
	_ plugin.ReadyHook = (*HttpRecord)(nil)
)

// NewHttpRecord 创建 HTTP 访问记录生命周期插件
func NewHttpRecord() *HttpRecord {
	return &HttpRecord{Base: plugin.NewBase(Name)}
}

// From 从插件插槽中取出 HTTP 访问记录插件；未注册时返回 nil
func From(slot *plugin.PluginSlot) *HttpRecord {
	if slot == nil {
		return nil
	}
	p, _ := slot.Plugin(Name)
	httpRecordPlugin, _ := p.(*HttpRecord)
	return httpRecordPlugin
}

// Init 按 provider 装配生产端并校验消费端依赖；消费在 OnReady 阶段启动
func (p *HttpRecord) Init(_ context.Context, slot *plugin.PluginSlot) (err error) {
	bootCfg := config.MustSection[BootConfig](slot.Config, SectionKey)
	if !bootCfg.EnabledOrDefault() {
		return plugin.ErrDisabled
	}

	cfg := bootCfg.ToConfig()
	p.cfg = cfg
	p.provider = cfg.ProviderOrDefault()
	p.runtimeCtx = slot.RuntimeContext()

	defer func() {
		if err != nil {
			p.cleanup()
		}
	}()

	if err = p.initProducer(slot); err != nil {
		return err
	}
	return p.initConsumerDeps(slot)
}

// initProducer 按 provider 装配生产端
func (p *HttpRecord) initProducer(slot *plugin.PluginSlot) error {
	switch p.provider {
	case httprecord.ProviderKafka:
		publisher := kafkaplugin.FromPublisher(slot).Service()
		if publisher == nil {
			return errors.New("http_record: provider=kafka 需要 kafka-publisher 插件")
		}
		p.service = httprecord.NewKafkaHttpRecordService(publisher, p.cfg.KafkaTopicOrDefault())
	case httprecord.ProviderMemory:
		p.memorySvc = httprecord.NewMemoryHttpRecordService(p.cfg.Memory.QueueSizeOrDefault())
		p.service = p.memorySvc
	case httprecord.ProviderRedisStream:
		publisher := redisstreamplugin.From(slot).Publisher()
		if publisher == nil {
			return errors.New("http_record: provider=redis_stream 需要 redisstream 插件")
		}
		p.service = httprecord.NewRedisStreamHttpRecordService(publisher, p.cfg.StreamOrDefault())
	default:
		return errors.Newf("http_record: 不支持的 provider: %s", p.provider)
	}
	return nil
}

// initConsumerDeps 在启用消费时解析并持有消费端服务；启动动作在 OnReady 执行
func (p *HttpRecord) initConsumerDeps(slot *plugin.PluginSlot) error {
	if !p.shouldConsume() {
		return nil
	}

	name := appName(slot)
	p.consumerGroup = resolveConsumerGroupID(p.cfg.Consumer.GroupID, name)

	switch p.provider {
	case httprecord.ProviderKafka:
		consumer := kafkaplugin.FromConsumer(slot).Service()
		if consumer == nil {
			return errors.New("http_record: 已启用消费但 kafka 消费者未初始化（检查 kafka.consumer_enabled）")
		}
		p.kafkaConsumer = consumer
	case httprecord.ProviderRedisStream:
		consumer := redisstreamplugin.From(slot).Consumer()
		if consumer == nil {
			return errors.New("http_record: 已启用消费但 redisstream 未初始化（检查 redis_stream.enabled）")
		}
		p.streamConsumer = consumer
		p.consumerName = redisConsumerName(name)
	case httprecord.ProviderMemory:
		// 内存队列自消费，无需外部依赖
	default:
		return errors.Newf("http_record: 不支持的 provider: %s", p.provider)
	}
	return nil
}

// Close 停止消费者并释放生产端；消费协程已随 RuntimeContext 取消退出
func (p *HttpRecord) Close(context.Context) error {
	if p == nil {
		return nil
	}

	var errs []error
	if p.kafkaConsumer != nil {
		if err := p.kafkaConsumer.Stop(httprecord.KafkaConsumerID); err != nil {
			errs = append(errs, errors.Wrap(err, "http_record: 停止 kafka 消费者失败"))
		}
		p.kafkaConsumer = nil
	}
	if p.streamConsumer != nil {
		if err := p.streamConsumer.Stop(httprecord.RedisStreamConsumerID); err != nil {
			errs = append(errs, errors.Wrap(err, "http_record: 停止 Redis Stream 消费者失败"))
		}
		p.streamConsumer = nil
	}
	p.cleanup()
	return errors.Join(errs...)
}

// cleanup 释放生产端资源；重复调用安全
func (p *HttpRecord) cleanup() {
	if p.memorySvc != nil {
		p.memorySvc.Close()
		p.memorySvc = nil
	}
	p.service = nil
}

// Service 返回 HTTP 访问记录服务；未启用时返回 nil
func (p *HttpRecord) Service() httprecord.HttpRecordService {
	if p == nil {
		return nil
	}
	return p.service
}
