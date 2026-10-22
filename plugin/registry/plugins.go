// 内置插件启动顺序。

package registry

import (
	"gitee.com/hq_project/yomi-boot/plugin"
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

// Plugins 按严格顺序返回全部内置插件；是否真正启动由各插件按配置 enabled 判断。
//
// # 为什么顺序很重要
//
// 后序插件 Init 时从 plugin.Host 读取前序插件写入的资源；顺序错误会导致 Init panic。
//
// # 启动顺序（不可随意调整）
//
//  1. app         — 始终启动，写入 Host.AppCtx，并按 app.timezone 锁定进程时区
//  2. logger      — 始终启动（依赖 AppCtx）
//  3. pprof       — pprof.enabled=true 时启动（默认 false）
//  4. db          — db.enabled=true 时启动（写入 Host.DbType）
//  5. redis       — redis.enabled=true 时启动
//  6. redisratelimit — 始终写入 RateLimiter（未启用时为 Noop；启用时依赖 RedisService）
//  7. kafka       — kafka.enabled=true 时启动
//  8. redisstream — redis_stream.enabled=true 时启动（依赖 RedisClient）
//  9. snowflake   — 始终启动
//  10. tx         — host.DB 存在时启动
//  11. consul     — consul.enabled=true 时启动（写入 Host.ConsulClient；Ready 注册 / Drain 注销）
//  12. dtm        — 构造 Host.DtmClient（注入 Host.ConsulClient）；enabled 时探活
//  13. idempotency — stale_cleaner_enabled=true 时启动
//  14. token      — auth.enabled=true 时启动服务；middleware 始终挂载，受 auth.enabled 控制
//  15. httprecord — http_record.enabled=true 时按 provider 装配 kafka/memory/redis_stream 服务；未启用时不创建服务（中间件短路）；默认关，文件 access 由 logger.Middleware 负责
//  16. event      — outbox_enabled / inbox_enabled 任一为 true 时启动（依赖 AppCtx/DbType 等运行态）
//  17. custom     — InitOptions.AddCustomPlugin 注册的服务自定义插件
//
// # 非插件（由 bootstrap.MustInit 直接处理，不在此列表）
//
//   - registry.DefaultConfigRegistry + MustLoadDocument（含自定义 AddConfigSection）
//
// Ready 按 Init 顺序执行，Drain 按 Init 逆序执行；仅 Started 插件参与（见 plugin.ReadyHook / DrainHook）。
// Close 按 Init 逆序：先停 custom 插件，再关 event，避免 Receiver 先于消费退出。
func Plugins() []plugin.Plugin {
	return []plugin.Plugin{
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
}
