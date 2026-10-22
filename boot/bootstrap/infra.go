// Package bootstrap 提供各服务共用的基础设施初始化与 HTTP 装配
//
// @author zwei
package bootstrap

import (
	"gitee.com/hq_project/yomi-boot/component/cloud/consul"
	"gitee.com/hq_project/yomi-boot/component/cloud/dtm"
	"gitee.com/hq_project/yomi-boot/component/common/appctx"
	"gitee.com/hq_project/yomi-boot/component/common/db"
	"gitee.com/hq_project/yomi-boot/component/common/distlock"
	"gitee.com/hq_project/yomi-boot/component/common/event"
	"gitee.com/hq_project/yomi-boot/component/common/httprecord"
	yomikafka "gitee.com/hq_project/yomi-boot/component/common/kafka"
	yomiredis "gitee.com/hq_project/yomi-boot/component/common/redis"
	"gitee.com/hq_project/yomi-boot/component/common/redisratelimit"
	"gitee.com/hq_project/yomi-boot/component/common/redisstream"
	"gitee.com/hq_project/yomi-boot/component/common/snowflake"
	"gitee.com/hq_project/yomi-boot/component/common/token"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/shared/enums"
	"gitee.com/hq_project/yomi-boot/plugin"

	"gorm.io/gorm"
)

// Infra 聚合已启动的基础设施运行时依赖；配置统一经 Doc 读取
type Infra struct {
	Doc *config.Document // 已绑定配置文档（含自定义段）

	DB                   *gorm.DB                     // 业务数据库连接
	DbType               enums.DbType                 // 数据库类型（db 插件写入）
	RedisService         yomiredis.RedisService       // Redis 访问服务，业务层经此访问 Redis
	DistLockFactory      distlock.DistLockFactory     // 分布式锁工厂，基于 Redis 实现
	RateLimiter          redisratelimit.LimitService  // 调用式限流（依赖 RedisService；未启用时为 Noop）
	KafkaPublisher       yomikafka.PublisherService   // Kafka 生产者
	KafkaConsumer        yomikafka.ConsumerService    // Kafka 消费者
	RedisStreamPublisher redisstream.PublisherService // Redis Stream 发布者
	RedisStreamConsumer  redisstream.ConsumerService  // Redis Stream 消费者
	IDGenerator          snowflake.Generator          // 雪花 ID 生成器
	TokenService         token.TokenService           // 令牌服务（auth.enabled=true 时可用）
	HttpRecordService    httprecord.HttpRecordService // HTTP 访问记录服务
	TxExecutor           db.TxExecutor                // 事务执行器
	AppCtx               appctx.AppCtx                // 应用上下文（名称、环境、版本）
	ConsulClient         *consul.Client               // Consul 客户端
	DtmClient            *dtm.Client                  // DTM 客户端
	EventComponents      *event.Components            // 可靠领域事件组件（Outbox/Inbox）

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
		Doc:                  doc,
		DB:                   host.DB,
		DbType:               host.DbType,
		RedisService:         host.RedisService,
		DistLockFactory:      host.DistLockFactory,
		RateLimiter:          host.RateLimiter,
		KafkaPublisher:       host.KafkaPublisher,
		KafkaConsumer:        host.KafkaConsumer,
		RedisStreamPublisher: host.RedisStreamPublisher,
		RedisStreamConsumer:  host.RedisStreamConsumer,
		IDGenerator:          host.IDGenerator,
		TokenService:         host.TokenService,
		HttpRecordService:    host.HttpRecordService,
		TxExecutor:           host.TxExecutor,
		AppCtx:               host.AppCtx,
		ConsulClient:         host.ConsulClient,
		DtmClient:            host.DtmClient,
		EventComponents:      host.EventComponents,
		runtime:              rt,
	}
}
