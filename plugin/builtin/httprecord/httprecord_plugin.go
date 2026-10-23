// Package httprecordplugin HTTP 访问记录插件
//
// @author zwei
package httprecordplugin

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/common/httprecord"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/plugin"
	kafkaplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/kafka"
	redisstreamplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/redisstream"
)

const Name = "httprecord"

// HttpRecord HTTP 访问记录插件；自包含按 provider 装配的记录服务
type HttpRecord struct {
	plugin.Base

	service   httprecord.HttpRecordService
	memorySvc *httprecord.MemoryHttpRecordService
}

var _ plugin.Plugin = (*HttpRecord)(nil)

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

// Init 按 provider 装配 HttpRecordService：未启用时不创建服务（中间件短路）
func (p *HttpRecord) Init(_ context.Context, slot *plugin.PluginSlot) error {
	bootCfg := config.MustSection[BootConfig](slot.Config, SectionKey)
	if !bootCfg.EnabledOrDefault() {
		return plugin.ErrDisabled
	}
	cfg := bootCfg.ToConfig()

	switch cfg.ProviderOrDefault() {
	case httprecord.ProviderKafka:
		publisher := kafkaplugin.FromPublisher(slot).Service()
		if publisher == nil {
			return errors.New("http_record: provider=kafka 需要 kafka-publisher 插件")
		}
		p.service = httprecord.NewKafkaHttpRecordService(publisher, cfg.KafkaTopicOrDefault())
	case httprecord.ProviderMemory:
		p.memorySvc = httprecord.NewMemoryHttpRecordService(cfg.Memory.QueueSizeOrDefault())
		p.service = p.memorySvc
	case httprecord.ProviderRedisStream:
		publisher := redisstreamplugin.From(slot).Publisher()
		if publisher == nil {
			return errors.New("http_record: provider=redis_stream 需要 redisstream 插件")
		}
		p.service = httprecord.NewRedisStreamHttpRecordService(publisher, cfg.StreamOrDefault())
	default:
		return errors.Newf("http_record: 不支持的 provider: %s", cfg.ProviderOrDefault())
	}
	return nil
}

// Close 关闭内存队列服务（若有）；不等待排空
func (p *HttpRecord) Close(context.Context) error {
	if p.memorySvc != nil {
		p.memorySvc.Close()
		p.memorySvc = nil
	}
	p.service = nil
	return nil
}

// Service 返回 HTTP 访问记录服务；未启用时返回 nil
func (p *HttpRecord) Service() httprecord.HttpRecordService {
	if p == nil {
		return nil
	}
	return p.service
}
