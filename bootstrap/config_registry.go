// Package bootstrap 默认配置段注册表
//
// @author zwei
package bootstrap

import (
	"gitee.com/hq_project/yomi-boot/appctx"
	"gitee.com/hq_project/yomi-boot/config"
	"gitee.com/hq_project/yomi-boot/consul"
	"gitee.com/hq_project/yomi-boot/cors"
	"gitee.com/hq_project/yomi-boot/db"
	"gitee.com/hq_project/yomi-boot/dtm"
	"gitee.com/hq_project/yomi-boot/event"
	"gitee.com/hq_project/yomi-boot/httprecord"
	"gitee.com/hq_project/yomi-boot/idempotency"
	yomikafka "gitee.com/hq_project/yomi-boot/kafka"
	"gitee.com/hq_project/yomi-boot/logger"
	"gitee.com/hq_project/yomi-boot/oss"
	bootpprof "gitee.com/hq_project/yomi-boot/pprof"
	"gitee.com/hq_project/yomi-boot/ratelimit"
	"gitee.com/hq_project/yomi-boot/redis"
	"gitee.com/hq_project/yomi-boot/redisstream"
	"gitee.com/hq_project/yomi-boot/router"
	"gitee.com/hq_project/yomi-boot/snowflake"
	"gitee.com/hq_project/yomi-boot/token"
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
			redisstream.ConfigSection(),
			token.ConfigSection(),
			cors.ConfigSection(),
			httprecord.ConfigSection(),
			consul.ConfigSection(),
			dtm.ConfigSection(),
			event.ConfigSection(),
			snowflake.ConfigSection(),
			idempotency.ConfigSection(),
			ratelimit.ConfigSection(),
			oss.ConfigSection(),
			bootpprof.ConfigSection(),
		)
}
