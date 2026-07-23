// Package httprecordplugin HTTP 访问记录插件
//
// @author zwei
package httprecordplugin

import (
	"gitee.com/hq_project/yomi-boot/config"
	"gitee.com/hq_project/yomi-boot/httprecord"
	"gitee.com/hq_project/yomi-boot/plugin"
)

// httpRecordPlugin HTTP 访问记录插件
type httpRecordPlugin struct {
	plugin.PluginState
}

const httpRecordName = "httprecord"

// NewHttpRecord 创建 HTTP 访问记录生命周期插件
func NewHttpRecord() plugin.Plugin {
	return &httpRecordPlugin{PluginState: plugin.NewPluginState(httpRecordName)}
}

// Init 装配 HttpRecordService：默认 Nop；enabled 时必须已有 KafkaPublisher
func (p *httpRecordPlugin) Init(host *plugin.Host) {
	cfg := config.MustSection[httprecord.Config](host.Doc, httprecord.SectionKey)
	if !cfg.EnabledOrDefault() {
		host.HttpRecordService = httprecord.NewNopService()
		p.SetStarted()
		return
	}
	if host.KafkaPublisher == nil {
		panic("http_record.enabled=true 但 KafkaPublisher 未初始化")
	}
	host.HttpRecordService = httprecord.NewKafkaHttpRecordService(
		host.KafkaPublisher,
		cfg.KafkaTopicOrDefault(),
	)
	p.SetStarted()
}

// Close 无额外资源需释放
func (p *httpRecordPlugin) Close(_ *plugin.Host) {}
