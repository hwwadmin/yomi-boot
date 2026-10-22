// 默认配置段注册表。

package registry

import (
	"gitee.com/hq_project/yomi-boot/component/common/oss"
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
	pprofplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/pprof"
	redisplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/redis"
	redisratelimitplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/redisratelimit"
	redisstreamplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/redisstream"
	routerplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/router"
	snowflakeplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/snowflake"
	tokenplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/token"
)

// Sections 按注册顺序返回全部内置配置段。
// oss 是规范允许的例外：业务直接装配，BootConfig 留在组件包。
func Sections() []config.Section {
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
		oss.ConfigSection(),
		pprofplugin.ConfigSection(),
	}
}

// DefaultConfigRegistry 返回注册全部内置配置段的注册表。
func DefaultConfigRegistry() *config.Registry {
	return config.NewRegistry().Register(Sections()...)
}
