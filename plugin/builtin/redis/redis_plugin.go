// Package redisplugin Redis 插件
//
// @author zwei
package redisplugin

import (
	"gitee.com/hq_project/yomi-boot/component/common/distlock"
	yomiredis "gitee.com/hq_project/yomi-boot/component/common/redis"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/plugin"
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
	bootCfg := config.MustSection[BootConfig](host.Config, SectionKey)
	if !bootCfg.EnabledOrDefault() {
		return
	}
	cfg := bootCfg.ToConfig()
	conn := yomiredis.MustNewClient(cfg)
	host.RedisClient = conn
	host.RedisService = yomiredis.NewRedisHelper(conn, yomiredis.WithCommandTimeout(cfg.CommandTimeout()))
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
