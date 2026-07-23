// Package bootstrap 基础设施插件注册表
//
// @author zwei
package bootstrap

import (
	appctxplugin "yt-boot/appctx/plugin"
	consulplugin "yt-boot/consul/plugin"
	dbplugin "yt-boot/db/plugin"
	dtmplugin "yt-boot/dtm/plugin"
	eventplugin "yt-boot/event/plugin"
	httprecordplugin "yt-boot/httprecord/plugin"
	idempotencyplugin "yt-boot/idempotency/plugin"
	kafkaplugin "yt-boot/kafka/plugin"
	loggerplugin "yt-boot/logger/plugin"
	"yt-boot/plugin"
	pprofplugin "yt-boot/pprof/plugin"
	ratelimitplugin "yt-boot/ratelimit/plugin"
	redisplugin "yt-boot/redis/plugin"
	snowflakeplugin "yt-boot/snowflake/plugin"
	tokenplugin "yt-boot/token/plugin"
)

// buildInfraPlugins 按严格顺序返回全部基础设施插件
//
// 是否真正启动由各自插件 Init 根据配置 enabled 自行判断；本函数只负责编排顺序
//
// # 为什么顺序很重要
//
// 后序插件 Init 时从 plugin.Host 读取前序插件写入的资源；顺序错误会导致 Init panic
//
// # 启动顺序（不可随意调整）
//
//  1. app         — 始终启动，写入 Host.AppCtx
//  2. logger      — 始终启动（依赖 AppCtx）
//  3. pprof       — pprof.enabled=true 时启动（默认 false）
//  4. db          — db.enabled=true 时启动（写入 Host.DbType）
//  5. redis       — redis.enabled=true 时启动
//  6. ratelimit   — 始终写入 RateLimiter（未启用时为 Noop；启用时依赖 RedisService）
//  7. kafka       — kafka.enabled=true 时启动
//  8. snowflake   — 始终启动
//  9. tx          — host.DB 存在时启动
//
// 10. dtm         — 写入 DtmServer；enabled 时探活
// 11. idempotency — stale_cleaner_enabled=true 时启动
// 12. token       — auth.enabled=true 时启动服务；middleware 始终挂载，受 auth.enabled 控制
// 13. httprecord  — http_record.enabled=true 时装配 Kafka 服务，否则 Nop（默认关；文件 access 由 logger.AccessMiddleware 负责）
// 14. consul      — consul.enabled=true 时启动
// 15. event       — publisher_enabled / consumer_enabled 任一为 true 时启动（依赖 AppCtx/DbType 等运行态）
// 16. custom      — InfraOptions.AddCustomPlugin 注册的服务自定义插件
//
// # 非插件（由 MustInitInfra 直接处理，不在此列表）
//
//   - DefaultConfigRegistry + MustLoadDocument（含自定义 AddConfigSection）
//
// Close 按 Init 逆序：先停 custom 插件，再关 event，避免 Receiver 先于消费退出。
func buildInfraPlugins(opts InfraOptions) []plugin.Plugin {
	plugins := []plugin.Plugin{
		appctxplugin.NewAppCtx(),
		loggerplugin.NewLogger(),
		pprofplugin.NewPprof(),
		dbplugin.NewDB(),
		redisplugin.NewRedis(),
		ratelimitplugin.NewRateLimit(),
		kafkaplugin.NewKafka(),
		snowflakeplugin.NewSnowflake(),
		dbplugin.NewTx(),
		dtmplugin.NewDtm(),
		idempotencyplugin.NewIdempotencyStaleCleaner(),
		tokenplugin.NewToken(),
		httprecordplugin.NewHttpRecord(),
		consulplugin.NewConsul(),
		eventplugin.NewEvent(),
	}
	plugins = append(plugins, opts.customPlugins...)
	return plugins
}
