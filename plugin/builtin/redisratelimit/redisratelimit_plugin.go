// Package redisratelimitplugin 限流插件
//
// @author zwei
package redisratelimitplugin

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/common/redisratelimit"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/plugin"
	redisplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/redis"
)

const Name = "redisratelimit"

// RedisRateLimit 限流插件；自包含限流器，未启用时为 Noop
type RedisRateLimit struct {
	plugin.Base

	limiter redisratelimit.LimitService
}

var _ plugin.Plugin = (*RedisRateLimit)(nil)

// NewRedisRateLimit 创建限流生命周期插件
func NewRedisRateLimit() *RedisRateLimit {
	return &RedisRateLimit{Base: plugin.NewBase(Name)}
}

// From 从插件插槽中取出限流插件；未注册时返回 nil
func From(slot *plugin.PluginSlot) *RedisRateLimit {
	if slot == nil {
		return nil
	}
	p, _ := slot.Plugin(Name)
	rateLimitPlugin, _ := p.(*RedisRateLimit)
	return rateLimitPlugin
}

// Init 初始化限流器；未启用时使用 Noop
func (p *RedisRateLimit) Init(_ context.Context, slot *plugin.PluginSlot) error {
	bootCfg := config.MustSection[BootConfig](slot.Config, SectionKey)
	if !bootCfg.EnabledOrDefault() {
		p.limiter = redisratelimit.NewNoop()
		return nil
	}
	redis := redisplugin.From(slot)
	if redis.Service() == nil {
		return errors.New("redisratelimit: 需要 redis 插件（redis.enabled=true）")
	}
	p.limiter = redisratelimit.New(redis.Service(), bootCfg.ToConfig())
	return nil
}

// Close 限流器无资源需要释放
func (p *RedisRateLimit) Close(context.Context) error { return nil }

// Limiter 返回限流服务；未初始化时返回 Noop
func (p *RedisRateLimit) Limiter() redisratelimit.LimitService {
	if p == nil || p.limiter == nil {
		return redisratelimit.NewNoop()
	}
	return p.limiter
}
