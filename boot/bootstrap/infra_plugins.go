// Package bootstrap 基础设施插件注册表
//
// @author zwei
package bootstrap

import (
	appctxplugin "gitee.com/hq_project/yomi-boot/boot/appctx/plugin"
	loggerplugin "gitee.com/hq_project/yomi-boot/boot/logger/plugin"
	"gitee.com/hq_project/yomi-boot/boot/plugin"
	pprofplugin "gitee.com/hq_project/yomi-boot/boot/pprof/plugin"
	snowflakeplugin "gitee.com/hq_project/yomi-boot/boot/snowflake/plugin"
	consulplugin "gitee.com/hq_project/yomi-boot/cloud/consul/plugin"
	dtmplugin "gitee.com/hq_project/yomi-boot/cloud/dtm/plugin"
	dbplugin "gitee.com/hq_project/yomi-boot/component/db/plugin"
	eventplugin "gitee.com/hq_project/yomi-boot/component/event/plugin"
	httprecordplugin "gitee.com/hq_project/yomi-boot/component/httprecord/plugin"
	idempotencyplugin "gitee.com/hq_project/yomi-boot/component/idempotency/plugin"
	kafkaplugin "gitee.com/hq_project/yomi-boot/component/kafka/plugin"
	redisplugin "gitee.com/hq_project/yomi-boot/component/redis/plugin"
	redisratelimitplugin "gitee.com/hq_project/yomi-boot/component/redisratelimit/plugin"
	redisstreamplugin "gitee.com/hq_project/yomi-boot/component/redisstream/plugin"
	tokenplugin "gitee.com/hq_project/yomi-boot/component/token/plugin"
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
//  1. app         — 始终启动，写入 Host.AppCtx，并按 app.timezone 锁定进程时区
//  2. logger      — 始终启动（依赖 AppCtx）
//  3. pprof       — pprof.enabled=true 时启动（默认 false）
//  4. db          — db.enabled=true 时启动（写入 Host.DbType）
//  5. redis        — redis.enabled=true 时启动
//  6. redisratelimit — 始终写入 RateLimiter（未启用时为 Noop；启用时依赖 RedisService）
//  7. kafka        — kafka.enabled=true 时启动
//  8. redisstream  — redis_stream.enabled=true 时启动（依赖 RedisClient）
//  9. snowflake    — 始终启动
//
// 10. tx           — host.DB 存在时启动
//
// 11. consul       — consul.enabled=true 时启动（写入 Host.ConsulClient）
// 12. dtm          — 构造 Host.DtmClient（注入 Host.ConsulClient）；enabled 时探活
// 13. idempotency  — stale_cleaner_enabled=true 时启动
// 14. token        — auth.enabled=true 时启动服务；middleware 始终挂载，受 auth.enabled 控制
// 15. httprecord   — http_record.enabled=true 时按 provider 装配 kafka/memory/redis_stream 服务，否则 Nop（默认关；文件 access 由 logger.AccessMiddleware 负责）
// 16. event        — outbox_enabled / inbox_enabled 任一为 true 时启动（依赖 AppCtx/DbType 等运行态）
// 17. custom       — InfraOptions.AddCustomPlugin 注册的服务自定义插件
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
		redisratelimitplugin.NewRedisRateLimit(),
		kafkaplugin.NewKafka(),
		redisstreamplugin.NewRedisStream(),
		snowflakeplugin.NewSnowflake(),
		dbplugin.NewTx(),
		consulplugin.NewConsul(),
		dtmplugin.NewDtm(),
		idempotencyplugin.NewIdempotencyStaleCleaner(),
		tokenplugin.NewToken(),
		httprecordplugin.NewHttpRecord(),
		eventplugin.NewEvent(),
	}
	plugins = append(plugins, opts.customPlugins...)
	return plugins
}
