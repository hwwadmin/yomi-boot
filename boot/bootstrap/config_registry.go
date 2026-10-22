// Package bootstrap 默认配置段注册表
//
// @author zwei
package bootstrap

import (
	"gitee.com/hq_project/yomi-boot/component/common/oss"
	"gitee.com/hq_project/yomi-boot/component/common/router"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/cors"
	appctxplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/appctx"
	consulplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/consul"
	dbplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/db"
	dtmplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/dtm"
	eventplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/event"
	httprecordplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/httprecord"
	idempotencyplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/idempotency"
	kafkaplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/kafka"
	loggerplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/logger"
	pprofplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/pprof"
	redisplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/redis"
	redisratelimitplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/redisratelimit"
	redisstreamplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/redisstream"
	snowflakeplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/snowflake"
	tokenplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/token"
)

// DefaultConfigRegistry 返回默认基础设施配置注册表
func DefaultConfigRegistry() *config.Registry {
	return config.NewRegistry().
		Register(
			appctxplugin.ConfigSection(),
			router.ConfigSection(),
			loggerplugin.ConfigSection(),
			dbplugin.ConfigSection(),
			redisplugin.ConfigSection(),
			kafkaplugin.ConfigSection(),
			redisstreamplugin.ConfigSection(),
			tokenplugin.ConfigSection(),
			cors.ConfigSection(),
			httprecordplugin.ConfigSection(),
			consulplugin.ConfigSection(),
			dtmplugin.ConfigSection(),
			eventplugin.ConfigSection(),
			snowflakeplugin.ConfigSection(),
			idempotencyplugin.ConfigSection(),
			redisratelimitplugin.ConfigSection(),
			oss.ConfigSection(),
			pprofplugin.ConfigSection(),
		)
}
