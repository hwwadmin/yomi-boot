// Package redisstreamplugin Redis Stream 插件
//
// @author zwei
package redisstreamplugin

import (
	"gitee.com/hq_project/yomi-boot/boot/plugin"
	"gitee.com/hq_project/yomi-boot/component/common/redisstream"
	"gitee.com/hq_project/yomi-boot/core/config"
)

// redisStreamPlugin Redis Stream 生命周期插件
type redisStreamPlugin struct {
	plugin.PluginState
	components *redisstream.Components // components 供 Close 时释放
}

// redisStreamName 是插件在生命周期中的名称
const redisStreamName = "redisstream"

// NewRedisStream 创建 Redis Stream 生命周期插件
func NewRedisStream() plugin.Plugin {
	return &redisStreamPlugin{PluginState: plugin.NewPluginState(redisStreamName)}
}

// Init 初始化 Redis Stream 组件并挂到 Host（复用 host.RedisClient，不耦合 RedisService）
func (p *redisStreamPlugin) Init(host *plugin.Host) {
	cfg := config.MustSection[redisstream.Config](host.Doc, redisstream.SectionKey)
	if !cfg.EnabledOrDefault() {
		return
	}
	if host.RedisClient == nil {
		panic("redis_stream.enabled=true 但 RedisClient 未初始化（请先启用 redis）")
	}
	components := redisstream.MustNewComponents(*cfg, host.RedisClient)
	p.components = components
	host.RedisStreamComponents = components
	host.RedisStreamPublisher = components.Publisher
	host.RedisStreamConsumer = components.Consumer
	p.SetStarted()
}

// Close 停止消费与发布侧资源；不关闭 Redis client
func (p *redisStreamPlugin) Close(_ *plugin.Host) {
	if p.components == nil {
		return
	}
	_ = p.components.Close()
	p.components = nil
}
