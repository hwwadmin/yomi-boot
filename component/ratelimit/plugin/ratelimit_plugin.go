// Package ratelimitplugin 限流插件
//
// @author zwei
package ratelimitplugin

import (
	"gitee.com/hq_project/yomi-boot/boot/config"
	"gitee.com/hq_project/yomi-boot/boot/plugin"
	"gitee.com/hq_project/yomi-boot/component/ratelimit"
)

// rateLimitPlugin 限流插件
type rateLimitPlugin struct {
	plugin.PluginState
}

const rateLimitName = "ratelimit"

// NewRateLimit 创建限流生命周期插件（依赖 RedisService；未启用时写入 Noop）
func NewRateLimit() plugin.Plugin {
	return &rateLimitPlugin{PluginState: plugin.NewPluginState(rateLimitName)}
}

// Init 初始化限流器并挂到 Host
func (p *rateLimitPlugin) Init(host *plugin.Host) {
	cfg := config.MustSection[ratelimit.Config](host.Doc, ratelimit.SectionKey)
	host.RateLimiter = ratelimit.New(host.RedisService, *cfg)
	p.SetStarted()
}

// Close 关闭限流资源
func (p *rateLimitPlugin) Close(_ *plugin.Host) {}
