// Package bootstrap 默认配置段注册表
//
// @author zwei
package bootstrap

import (
	"gitee.com/hq_project/yomi-boot/component/cloud/consul"
	"gitee.com/hq_project/yomi-boot/component/cloud/dtm"
	"gitee.com/hq_project/yomi-boot/component/common/appctx"
	"gitee.com/hq_project/yomi-boot/component/common/db"
	"gitee.com/hq_project/yomi-boot/component/common/event"
	"gitee.com/hq_project/yomi-boot/component/common/httprecord"
	"gitee.com/hq_project/yomi-boot/component/common/idempotency"
	yomikafka "gitee.com/hq_project/yomi-boot/component/common/kafka"
	"gitee.com/hq_project/yomi-boot/component/common/logger"
	"gitee.com/hq_project/yomi-boot/component/common/oss"
	componentpprof "gitee.com/hq_project/yomi-boot/component/common/pprof"
	"gitee.com/hq_project/yomi-boot/component/common/redis"
	"gitee.com/hq_project/yomi-boot/component/common/redisratelimit"
	"gitee.com/hq_project/yomi-boot/component/common/redisstream"
	"gitee.com/hq_project/yomi-boot/component/common/router"
	"gitee.com/hq_project/yomi-boot/component/common/snowflake"
	"gitee.com/hq_project/yomi-boot/component/common/token"
	"gitee.com/hq_project/yomi-boot/core/config"
	"gitee.com/hq_project/yomi-boot/core/cors"
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
			componentpprof.ConfigSection(),
		)
}
