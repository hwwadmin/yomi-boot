// Services 定义进程启动后业务装配可见的能力集合。

package plugin

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

	"gorm.io/gorm"
)

// Services 是进程启动后各插件写入、业务装配只读的能力集合。
// 配置统一经 Doc 读取；未启用的能力保持零值。
type Services struct {
	Doc *config.Document // 已绑定配置文档（含自定义段）

	AppCtx               appctx.AppCtx                // 应用上下文（名称、环境、版本）
	DbType               enums.DbType                 // 数据库类型（db 插件写入）
	DB                   *gorm.DB                     // 业务数据库连接
	RedisService         yomiredis.RedisService       // Redis 访问服务，业务层经此访问 Redis
	DistLockFactory      distlock.DistLockFactory     // 分布式锁工厂，基于 Redis 实现
	RateLimiter          redisratelimit.LimitService  // 调用式限流（未启用时为 Noop）
	KafkaPublisher       yomikafka.PublisherService   // Kafka 生产者
	KafkaConsumer        yomikafka.ConsumerService    // Kafka 消费者
	RedisStreamPublisher redisstream.PublisherService // Redis Stream 发布者
	RedisStreamConsumer  redisstream.ConsumerService  // Redis Stream 消费者
	IDGenerator          snowflake.Generator          // 雪花 ID 生成器
	TokenService         token.TokenService           // 令牌服务（auth.enabled=true 时可用）
	HttpRecordService    httprecord.HttpRecordService // HTTP 访问记录服务
	TxExecutor           db.TxExecutor                // 事务执行器
	ConsulClient         *consul.Client               // Consul 客户端
	DtmClient            *dtm.Client                  // DTM 客户端
	EventComponents      *event.Components            // 可靠领域事件组件（Outbox/Inbox）
}
