// Package redisstreamplugin Redis Stream 插件
//
// @author zwei
package redisstreamplugin

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/common/redisstream"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/plugin"
	redisplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/redis"
)

const Name = "redisstream"

// RedisStream Redis Stream 生命周期插件；自包含发布/消费组件
type RedisStream struct {
	plugin.Base

	components *redisstream.Components
}

var _ plugin.Plugin = (*RedisStream)(nil)

// NewRedisStream 创建 Redis Stream 生命周期插件
func NewRedisStream() *RedisStream {
	return &RedisStream{Base: plugin.NewBase(Name)}
}

// From 从插件插槽中取出 Redis Stream 插件；未注册时返回 nil
func From(slot *plugin.PluginSlot) *RedisStream {
	if slot == nil {
		return nil
	}
	p, _ := slot.Plugin(Name)
	redisStreamPlugin, _ := p.(*RedisStream)
	return redisStreamPlugin
}

// Init 初始化 Redis Stream 组件，复用前序 redis 插件的原始客户端
func (p *RedisStream) Init(_ context.Context, slot *plugin.PluginSlot) error {
	bootCfg := config.MustSection[BootConfig](slot.Config, SectionKey)
	if !bootCfg.EnabledOrDefault() {
		return plugin.ErrDisabled
	}
	redis := redisplugin.From(slot)
	if redis.Client() == nil {
		return errors.New("redis_stream: 需要 redis 插件（redis.enabled=true）")
	}
	components, err := redisstream.NewComponents(bootCfg.ToConfig(), redis.Client())
	if err != nil {
		return err
	}
	p.components = components
	return nil
}

// Close 停止消费与发布侧资源；不关闭 Redis client
func (p *RedisStream) Close(context.Context) error {
	if p.components == nil {
		return nil
	}
	err := p.components.Close()
	p.components = nil
	return err
}

// Publisher 返回 Redis Stream 发布者；未启用时返回 nil
func (p *RedisStream) Publisher() redisstream.PublisherService {
	if p == nil || p.components == nil {
		return nil
	}
	return p.components.Publisher
}

// Consumer 返回 Redis Stream 消费者；未启用时返回 nil
func (p *RedisStream) Consumer() redisstream.ConsumerService {
	if p == nil || p.components == nil {
		return nil
	}
	return p.components.Consumer
}
