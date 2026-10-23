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
	ossplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/oss"
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
// 后序插件在 Init 中通过 PluginSlot 按名称取到前序插件，并读取其自包含组件；
// 顺序错误会导致依赖检查失败并 panic。
//
// # 启动顺序（不可随意调整）
//
//  1. app         — 始终启动；提供 AppCtx 并按 app.timezone 锁定进程时区
//  2. logger      — 始终启动（依赖 app 插件）
//  3. pprof       — pprof.enabled=true 时启动（默认 false）
//  4. db          — db.enabled=true 时启动（提供 DB 与 DbType）
//  5. redis       — redis.enabled=true 时启动（提供 Client / RedisService / DistLockFactory）
//  6. redisratelimit — 始终启动（未启用时 Limiter 为 Noop；启用时依赖 redis 插件）
//  7. kafka-publisher — kafka.publisher_enabled=true 时启动
//  8. kafka-consumer  — kafka.consumer_enabled=true 时启动
//  9. redisstream — redis_stream.enabled=true 时启动（依赖 redis 插件 Client）
//  10. snowflake  — 始终启动
//  11. oss        — oss.enabled=true 时启动
//  12. tx         — db 插件已启用时启动
//  13. consul     — consul.enabled=true 时启动（Ready 注册 / Drain 注销）
//  14. dtm        — 始终构造客户端（注入 consul 插件 Client）；enabled 时探活
//  15. idempotency — stale_cleaner_enabled=true 时启动（依赖 app/db/redis 插件）
//  16. token      — auth.enabled=true 时启动服务；middleware 始终挂载，受 auth.enabled 控制
//  17. httprecord — http_record.enabled=true 时按 provider 装配 kafka/memory/redis_stream 服务；未启用时不创建服务（中间件短路）；默认关，文件 access 由 logger.Middleware 负责
//  18. event      — outbox_enabled / inbox_enabled 任一为 true 时启动（依赖 app/db/tx/redis/kafka/snowflake 插件）
//  19. custom     — PluginOptions.AddCustomPlugin 注册的服务自定义插件
//
// # 非插件（由 boot.MustInit 直接处理，不在此列表）
//
//   - registry.DefaultConfigRegistry + MustLoadConfig（含 PluginOptions 注册的自定义配置段）
//
// Ready 按 Init 顺序执行，Drain 按 Init 逆序执行；仅 Started 插件参与（见 plugin.ReadyHook / DrainHook）。
// Close 按 Init 逆序：先停 custom 插件，再关 event，随后 kafka-consumer 先于 kafka-publisher 关闭，
// 避免消费侧收尾时仍需投递却发现生产者已关闭。
func Plugins() []plugin.Plugin {
	return []plugin.Plugin{
		appctxplugin.NewAppCtx(),
		loggerplugin.NewLogger(),
		pprofplugin.NewPprof(),
		dbplugin.NewDB(),
		redisplugin.NewRedis(),
		redisratelimitplugin.NewRedisRateLimit(),
		kafkaplugin.NewKafkaPublisher(),
		kafkaplugin.NewKafkaConsumer(),
		redisstreamplugin.NewRedisStream(),
		snowflakeplugin.NewSnowflake(),
		ossplugin.NewOSS(),
		dbplugin.NewTx(),
		consulplugin.NewConsul(),
		dtmplugin.NewDtm(),
		idempotencyplugin.NewIdempotencyStaleCleaner(),
		tokenplugin.NewToken(),
		httprecordplugin.NewHttpRecord(),
		eventplugin.NewEvent(),
	}
}
