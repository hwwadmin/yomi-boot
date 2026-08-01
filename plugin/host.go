// Package plugin 初始化阶段共享上下文
//
// @author zwei
package plugin

import (
	"gitee.com/hq_project/yomi-boot/appctx"
	"gitee.com/hq_project/yomi-boot/config"
	"gitee.com/hq_project/yomi-boot/consul"
	"gitee.com/hq_project/yomi-boot/db"
	"gitee.com/hq_project/yomi-boot/distlock"
	"gitee.com/hq_project/yomi-boot/event"
	"gitee.com/hq_project/yomi-boot/httprecord"
	yomikafka "gitee.com/hq_project/yomi-boot/kafka"
	"gitee.com/hq_project/yomi-boot/ratelimit"
	yomiredis "gitee.com/hq_project/yomi-boot/redis"
	"gitee.com/hq_project/yomi-boot/redisstream"
	"gitee.com/hq_project/yomi-boot/shared/enums"
	"gitee.com/hq_project/yomi-boot/snowflake"
	"gitee.com/hq_project/yomi-boot/token"

	redisv9 "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// Host 插件 Init 阶段共享上下文：Doc 供各插件自取本段配置；其余为运行时产物。
// MustInitInfra 成功后映射为 bootstrap.Infra。
type Host struct {
	Doc *config.Document

	AppCtx appctx.AppCtx // 由 app 插件写入
	DbType enums.DbType  // 由 db 插件写入（db 未启用时为零值）

	DB                    *gorm.DB
	RedisClient           *redisv9.Client
	RedisService          yomiredis.RedisService
	DistLockFactory       distlock.DistLockFactory
	RateLimiter           ratelimit.LimitService
	KafkaComponents       *yomikafka.Components
	KafkaPublisher        yomikafka.PublisherService
	KafkaConsumer         yomikafka.ConsumerService
	RedisStreamComponents *redisstream.Components
	RedisStreamPublisher  redisstream.PublisherService
	RedisStreamConsumer   redisstream.ConsumerService
	IDGenerator           snowflake.Generator
	TokenService          token.TokenService
	HttpRecordService     httprecord.HttpRecordService
	TxExecutor            db.TxExecutor
	ConsulClient          *consul.Client
	ConsulServiceID       string
	DtmServer             string
	EventComponents       *event.Components
}
