// Package bootstrap 默认配置段注册表
//
// @author zwei
package bootstrap

import (
	"yomi-boot/appctx"
	"yomi-boot/config"
	"yomi-boot/consul"
	"yomi-boot/cors"
	"yomi-boot/db"
	"yomi-boot/dtm"
	"yomi-boot/event"
	"yomi-boot/filestorage"
	"yomi-boot/httprecord"
	"yomi-boot/idempotency"
	yomikafka "yomi-boot/kafka"
	"yomi-boot/logger"
	bootpprof "yomi-boot/pprof"
	"yomi-boot/ratelimit"
	"yomi-boot/redis"
	"yomi-boot/router"
	"yomi-boot/snowflake"
	"yomi-boot/token"
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
			yomikafka.ConfigSection(),
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
