// Package redisratelimitplugin 限流插件
//
// @author zwei
package redisratelimitplugin

import (
	"gitee.com/hq_project/yomi-boot/boot/plugin"
	"gitee.com/hq_project/yomi-boot/component/redisratelimit"
	"gitee.com/hq_project/yomi-boot/core/config"
)

// redisRateLimitPlugin 限流插件
type redisRateLimitPlugin struct {
	plugin.PluginState
}

const redisRateLimitName = "redisratelimit"

// NewRedisRateLimit 创建限流生命周期插件（依赖 RedisService；未启用时写入 Noop）
func NewRedisRateLimit() plugin.Plugin {
	return &redisRateLimitPlugin{PluginState: plugin.NewPluginState(redisRateLimitName)}
}

// Init 初始化限流器并挂到 Host
func (p *redisRateLimitPlugin) Init(host *plugin.Host) {
	cfg := config.MustSection[redisratelimit.Config](host.Doc, redisratelimit.SectionKey)
	host.RateLimiter = redisratelimit.New(host.RedisService, *cfg)
	p.SetStarted()
}

// Close 关闭限流资源
func (p *redisRateLimitPlugin) Close(_ *plugin.Host) {}
