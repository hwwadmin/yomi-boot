// 默认配置段。

package registry

import (
	"gitee.com/hq_project/yomi-boot/component/core/config"
	appctxplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/appctx"
	consulplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/consul"
	corsplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/cors"
	dbplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/db"
	dtmplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/dtm"
	eventplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/event"
	httprecordplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/httprecord"
	idempotencyplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/idempotency"
	kafkaplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/kafka"
	loggerplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/logger"
	ossplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/oss"
	pprofplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/pprof"
	redisplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/redis"
	redisratelimitplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/redisratelimit"
	redisstreamplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/redisstream"
	routerplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/router"
	snowflakeplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/snowflake"
	tokenplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/token"
)

// DefaultConfigs 按注册顺序返回全部内置配置段。
func DefaultConfigs() []config.Section {
	return []config.Section{
		appctxplugin.ConfigSection(),
		routerplugin.ConfigSection(),
		loggerplugin.ConfigSection(),
		dbplugin.ConfigSection(),
		redisplugin.ConfigSection(),
		kafkaplugin.ConfigSection(),
		redisstreamplugin.ConfigSection(),
		tokenplugin.ConfigSection(),
		corsplugin.ConfigSection(),
		httprecordplugin.ConfigSection(),
		consulplugin.ConfigSection(),
		dtmplugin.ConfigSection(),
		eventplugin.ConfigSection(),
		snowflakeplugin.ConfigSection(),
		idempotencyplugin.ConfigSection(),
		redisratelimitplugin.ConfigSection(),
		ossplugin.ConfigSection(),
		pprofplugin.ConfigSection(),
	}
}
