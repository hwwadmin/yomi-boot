// Package bootstrap 默认配置段注册表
//
// @author zwei
package bootstrap

import (
	"gitee.com/hq_project/yomi-boot/boot/appctx"
	"gitee.com/hq_project/yomi-boot/boot/config"
	"gitee.com/hq_project/yomi-boot/boot/cors"
	"gitee.com/hq_project/yomi-boot/boot/logger"
	bootpprof "gitee.com/hq_project/yomi-boot/boot/pprof"
	"gitee.com/hq_project/yomi-boot/boot/router"
	"gitee.com/hq_project/yomi-boot/boot/snowflake"
	"gitee.com/hq_project/yomi-boot/cloud/consul"
	"gitee.com/hq_project/yomi-boot/cloud/dtm"
	"gitee.com/hq_project/yomi-boot/component/db"
	"gitee.com/hq_project/yomi-boot/component/event"
	"gitee.com/hq_project/yomi-boot/component/httprecord"
	"gitee.com/hq_project/yomi-boot/component/idempotency"
	yomikafka "gitee.com/hq_project/yomi-boot/component/kafka"
	"gitee.com/hq_project/yomi-boot/component/oss"
	"gitee.com/hq_project/yomi-boot/component/redis"
	"gitee.com/hq_project/yomi-boot/component/redisratelimit"
	"gitee.com/hq_project/yomi-boot/component/redisstream"
	"gitee.com/hq_project/yomi-boot/component/token"
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
			redisratelimit.ConfigSection(),
			oss.ConfigSection(),
			bootpprof.ConfigSection(),
		)
}
