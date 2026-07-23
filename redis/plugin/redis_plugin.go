// Package redisplugin Redis 插件
//
// @author zwei
package redisplugin

import (
	"yt-boot/config"
	"yt-boot/distlock"
	"yt-boot/plugin"
	ytredis "yt-boot/redis"
)

// redisPlugin Redis 插件
type redisPlugin struct {
	plugin.PluginState
}

const redisName = "redis"

// NewRedis 创建 Redis 生命周期插件（含 RedisService 与 DistLockFactory）
func NewRedis() plugin.Plugin {
	return &redisPlugin{PluginState: plugin.NewPluginState(redisName)}
}

// Init 初始化 Redis 客户端并挂到 Host
func (p *redisPlugin) Init(host *plugin.Host) {
	cfg := config.MustSection[ytredis.Config](host.Doc, ytredis.SectionKey)
	if !cfg.EnabledOrDefault() {
		return
	}
	conn := ytredis.MustNewClient(*cfg)
	host.RedisClient = conn
	host.RedisService = ytredis.NewRedisHelper(conn)
	host.DistLockFactory = distlock.MustNewFactory(conn)
	p.SetStarted()
}

// Close 关闭 Redis 连接资源
func (p *redisPlugin) Close(host *plugin.Host) {
	if host.RedisClient == nil {
		return
	}
	_ = host.RedisClient.Close()
}
