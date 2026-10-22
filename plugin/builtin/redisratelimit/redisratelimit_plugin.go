// Package redisratelimitplugin 限流插件
//
// @author zwei
package redisratelimitplugin

import (
	"gitee.com/hq_project/yomi-boot/component/common/redisratelimit"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/plugin"
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
	bootCfg := config.MustSection[BootConfig](host.Doc, SectionKey)
	if !bootCfg.EnabledOrDefault() {
		host.RateLimiter = redisratelimit.NewNoop()
		p.SetStarted()
		return
	}
	host.RateLimiter = redisratelimit.New(host.RedisService, bootCfg.ToConfig())
	p.SetStarted()
}

// Close 关闭限流资源
func (p *redisRateLimitPlugin) Close(_ *plugin.Host) {}
