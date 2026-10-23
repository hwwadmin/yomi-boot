// Package redisplugin Redis 插件
//
// @author zwei
package redisplugin

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/common/distlock"
	yomiredis "gitee.com/hq_project/yomi-boot/component/common/redis"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/plugin"

	redisv9 "github.com/redis/go-redis/v9"
)

const Name = "redis"

// Redis Redis 插件；自包含客户端、RedisService 与分布式锁工厂
type Redis struct {
	plugin.Base

	client   *redisv9.Client
	service  yomiredis.RedisService
	distLock distlock.DistLockFactory
}

var _ plugin.Plugin = (*Redis)(nil)

// NewRedis 创建 Redis 生命周期插件
func NewRedis() *Redis {
	return &Redis{Base: plugin.NewBase(Name)}
}

// From 从插件插槽中取出 Redis 插件；未注册时返回 nil
func From(slot *plugin.PluginSlot) *Redis {
	if slot == nil {
		return nil
	}
	p, _ := slot.Plugin(Name)
	redisPlugin, _ := p.(*Redis)
	return redisPlugin
}

// Init 初始化 Redis 客户端、RedisService 与分布式锁工厂
func (p *Redis) Init(ctx context.Context, slot *plugin.PluginSlot) error {
	bootCfg := config.MustSection[BootConfig](slot.Config, SectionKey)
	if !bootCfg.EnabledOrDefault() {
		return plugin.ErrDisabled
	}
	cfg := bootCfg.ToConfig()
	client, err := yomiredis.NewClient(ctx, cfg)
	if err != nil {
		return err
	}
	distLock, err := distlock.NewFactory(client)
	if err != nil {
		_ = client.Close()
		return err
	}
	p.client = client
	p.service = yomiredis.NewRedisHelper(client, yomiredis.WithCommandTimeout(cfg.CommandTimeout()))
	p.distLock = distLock
	return nil
}

// Close 关闭 Redis 连接资源
func (p *Redis) Close(context.Context) error {
	if p.client == nil {
		return nil
	}
	err := p.client.Close()
	p.client = nil
	p.service = nil
	p.distLock = nil
	return err
}

// Client 返回 Redis 原始客户端，仅供插件初始化期复用；业务层应使用 Service
func (p *Redis) Client() *redisv9.Client {
	if p == nil {
		return nil
	}
	return p.client
}

// Service 返回 Redis 访问服务；未启用时返回 nil
func (p *Redis) Service() yomiredis.RedisService {
	if p == nil {
		return nil
	}
	return p.service
}

// DistLockFactory 返回分布式锁工厂；未启用时返回 nil
func (p *Redis) DistLockFactory() distlock.DistLockFactory {
	if p == nil {
		return nil
	}
	return p.distLock
}
