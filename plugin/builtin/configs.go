// 默认配置段。

package builtin

import (
	"github.com/hwwadmin/yomi-boot/component/core/config"
	appctxplugin "github.com/hwwadmin/yomi-boot/plugin/builtin/appctx"
	corsplugin "github.com/hwwadmin/yomi-boot/plugin/builtin/cors"
	dbplugin "github.com/hwwadmin/yomi-boot/plugin/builtin/db"
	eventplugin "github.com/hwwadmin/yomi-boot/plugin/builtin/event"
	httprecordplugin "github.com/hwwadmin/yomi-boot/plugin/builtin/httprecord"
	idempotencyplugin "github.com/hwwadmin/yomi-boot/plugin/builtin/idempotency"
	kafkaplugin "github.com/hwwadmin/yomi-boot/plugin/builtin/kafka"
	loggerplugin "github.com/hwwadmin/yomi-boot/plugin/builtin/logger"
	maskingplugin "github.com/hwwadmin/yomi-boot/plugin/builtin/masking"
	ossplugin "github.com/hwwadmin/yomi-boot/plugin/builtin/oss"
	pprofplugin "github.com/hwwadmin/yomi-boot/plugin/builtin/pprof"
	redisplugin "github.com/hwwadmin/yomi-boot/plugin/builtin/redis"
	redisratelimitplugin "github.com/hwwadmin/yomi-boot/plugin/builtin/redisratelimit"
	redisstreamplugin "github.com/hwwadmin/yomi-boot/plugin/builtin/redisstream"
	routerplugin "github.com/hwwadmin/yomi-boot/plugin/builtin/router"
	snowflakeplugin "github.com/hwwadmin/yomi-boot/plugin/builtin/snowflake"
	tokenplugin "github.com/hwwadmin/yomi-boot/plugin/builtin/token"
)

// DefaultConfigs 按注册顺序返回全部内置配置段。
func DefaultConfigs() []config.Section {
	return []config.Section{
		appctxplugin.ConfigSection(),
		maskingplugin.ConfigSection(),
		routerplugin.ConfigSection(),
		loggerplugin.ConfigSection(),
		dbplugin.ConfigSection(),
		redisplugin.ConfigSection(),
		kafkaplugin.ConfigSection(),
		redisstreamplugin.ConfigSection(),
		tokenplugin.ConfigSection(),
		corsplugin.ConfigSection(),
		httprecordplugin.ConfigSection(),
		eventplugin.ConfigSection(),
		snowflakeplugin.ConfigSection(),
		idempotencyplugin.ConfigSection(),
		redisratelimitplugin.ConfigSection(),
		ossplugin.ConfigSection(),
		pprofplugin.ConfigSection(),
	}
}
