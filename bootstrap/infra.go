// Package bootstrap 提供各服务共用的基础设施初始化与 HTTP 装配
//
// @author zwei
package bootstrap

import (
	"yomi-boot/appctx"
	"yomi-boot/config"
	"yomi-boot/consul"
	"yomi-boot/db"
	"yomi-boot/distlock"
	"yomi-boot/event"
	"yomi-boot/httprecord"
	ytkafka "yomi-boot/kafka"
	"yomi-boot/plugin"
	"yomi-boot/ratelimit"
	yomiredis "yomi-boot/redis"
	"yomi-boot/shared/enums"
	"yomi-boot/snowflake"
	"yomi-boot/token"

	"gorm.io/gorm"
)

// Infra 聚合已启动的基础设施运行时依赖；配置统一经 Doc 读取
type Infra struct {
	Doc *config.Document // 已绑定配置文档（含自定义段）

	DB                *gorm.DB                     // 业务数据库连接
	DbType            enums.DbType                 // 数据库类型（db 插件写入）
	RedisService      yomiredis.RedisService       // Redis 访问服务，业务层经此访问 Redis
	DistLockFactory   distlock.DistLockFactory     // 分布式锁工厂，基于 Redis 实现
	RateLimiter       ratelimit.LimitService       // 调用式限流（依赖 RedisService；未启用时为 Noop）
	KafkaPublisher    ytkafka.PublisherService     // Kafka 生产者
	KafkaConsumer     ytkafka.ConsumerService      // Kafka 消费者
	IDGenerator       snowflake.Generator          // 雪花 ID 生成器
	TokenService      token.TokenService           // 令牌服务（auth.enabled=true 时可用）
	HttpRecordService httprecord.HttpRecordService // HTTP 访问记录服务
	TxExecutor        db.TxExecutor                // 事务执行器
	AppCtx            appctx.AppCtx                // 应用上下文（名称、环境、版本）
	ConsulClient      *consul.Client               // Consul 客户端
	DtmServer         string                       // DTM 服务端地址
	EventComponents   *event.Components            // 消息队列事件组件

	runtime *plugin.Runtime // 插件运行时，供 Cleanup 逆序释放资源
}

// MustInitInfra 初始化基础设施：注册配置段 → 加载校验 → 插件生命周期；失败时 panic
func MustInitInfra(configPath, envPrefix string, opts InfraOptions) *Infra {
	reg := DefaultConfigRegistry()
	for _, s := range opts.configSections {
		reg.Register(s)
	}
	doc := config.MustLoadDocument(configPath, envPrefix, reg)

	host := &plugin.Host{Doc: doc}
	rt := plugin.Run(host, buildInfraPlugins(opts))

	return newInfra(doc, rt)
}

// Cleanup 逆序释放基础设施资源，失败静默跳过
func (infra *Infra) Cleanup() {
	if infra == nil || infra.runtime == nil {
		return
	}
	infra.runtime.Close()
	infra.runtime = nil
}

// newInfra 从插件运行时构造 Infra 聚合对象（仅透传 Host 运行态）
func newInfra(doc *config.Document, rt *plugin.Runtime) *Infra {
	host := rt.Host
	return &Infra{
		Doc:               doc,
		DB:                host.DB,
		DbType:            host.DbType,
		RedisService:      host.RedisService,
		DistLockFactory:   host.DistLockFactory,
		RateLimiter:       host.RateLimiter,
		KafkaPublisher:    host.KafkaPublisher,
		KafkaConsumer:     host.KafkaConsumer,
		IDGenerator:       host.IDGenerator,
		TokenService:      host.TokenService,
		HttpRecordService: host.HttpRecordService,
		TxExecutor:        host.TxExecutor,
		AppCtx:            host.AppCtx,
		ConsulClient:      host.ConsulClient,
		DtmServer:         host.DtmServer,
		EventComponents:   host.EventComponents,
		runtime:           rt,
	}
}
