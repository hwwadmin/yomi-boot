// Package httprecordplugin HTTP 访问记录插件
//
// @author zwei
package httprecordplugin

import (
	"gitee.com/hq_project/yomi-boot/boot/plugin"
	"gitee.com/hq_project/yomi-boot/component/httprecord"
	"gitee.com/hq_project/yomi-boot/core/config"
)

// httpRecordPlugin HTTP 访问记录插件
type httpRecordPlugin struct {
	plugin.PluginState
	memorySvc *httprecord.MemoryHttpRecordService
}

const httpRecordName = "httprecord"

// NewHttpRecord 创建 HTTP 访问记录生命周期插件
func NewHttpRecord() plugin.Plugin {
	return &httpRecordPlugin{PluginState: plugin.NewPluginState(httpRecordName)}
}

// Init 装配 HttpRecordService：默认 Nop；enabled 时按 provider 分发 kafka / memory / redis_stream
func (p *httpRecordPlugin) Init(host *plugin.Host) {
	cfg := config.MustSection[httprecord.Config](host.Doc, httprecord.SectionKey)
	if !cfg.EnabledOrDefault() {
		host.HttpRecordService = httprecord.NewNopService()
		p.SetStarted()
		return
	}

	switch cfg.ProviderOrDefault() {
	case httprecord.ProviderKafka:
		if host.KafkaPublisher == nil {
			panic("http_record.enabled=true 且 provider=kafka 但 KafkaPublisher 未初始化")
		}
		host.HttpRecordService = httprecord.NewKafkaHttpRecordService(
			host.KafkaPublisher,
			cfg.KafkaTopicOrDefault(),
		)
	case httprecord.ProviderMemory:
		svc := httprecord.NewMemoryHttpRecordService(cfg.Memory.QueueSizeOrDefault())
		p.memorySvc = svc
		host.HttpRecordService = svc
	case httprecord.ProviderRedisStream:
		if host.RedisStreamPublisher == nil {
			panic("http_record.enabled=true 且 provider=redis_stream 但 RedisStreamPublisher 未初始化")
		}
		host.HttpRecordService = httprecord.NewRedisStreamHttpRecordService(
			host.RedisStreamPublisher,
			cfg.StreamOrDefault(),
		)
	default:
		panic("http_record.provider 不支持: " + cfg.ProviderOrDefault())
	}
	p.SetStarted()
}

// Close 关闭内存队列服务（若有）；不等待排空
func (p *httpRecordPlugin) Close(_ *plugin.Host) {
	if p.memorySvc == nil {
		return
	}
	p.memorySvc.Close()
	p.memorySvc = nil
}
