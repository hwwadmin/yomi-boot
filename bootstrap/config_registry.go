// Package bootstrap 默认配置段注册表
//
// @author zwei
package bootstrap

import (
	"yt-boot/appctx"
	"yt-boot/config"
	"yt-boot/consul"
	"yt-boot/cors"
	"yt-boot/db"
	"yt-boot/dtm"
	"yt-boot/event"
	"yt-boot/filestorage"
	"yt-boot/httprecord"
	"yt-boot/idempotency"
	ytkafka "yt-boot/kafka"
	"yt-boot/logger"
	bootpprof "yt-boot/pprof"
	"yt-boot/ratelimit"
	"yt-boot/redis"
	"yt-boot/router"
	"yt-boot/snowflake"
	"yt-boot/token"
)

// DefaultConfigRegistry 返回默认基础设施配置注册表
func DefaultConfigRegistry() *config.Registry {
	return config.NewRegistry().
		Register(
			appctx.ConfigSection(),
			router.ConfigSection(),
			logger.ConfigSection(),
			db.ConfigSection(),
			redis.ConfigSection(),
			ytkafka.ConfigSection(),
			token.ConfigSection(),
			cors.ConfigSection(),
			httprecord.ConfigSection(),
			consul.ConfigSection(),
			dtm.ConfigSection(),
			event.ConfigSection(),
			snowflake.ConfigSection(),
			idempotency.ConfigSection(),
			ratelimit.ConfigSection(),
			filestorage.ConfigSection(),
			bootpprof.ConfigSection(),
		)
}
