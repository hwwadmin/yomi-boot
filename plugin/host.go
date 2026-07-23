// Package plugin 初始化阶段共享上下文
//
// @author zwei
package plugin

import (
	"yomi-boot/appctx"
	"yomi-boot/config"
	"yomi-boot/consul"
	"yomi-boot/db"
	"yomi-boot/distlock"
	"yomi-boot/event"
	"yomi-boot/httprecord"
	yomikafka "yomi-boot/kafka"
	"yomi-boot/ratelimit"
	yomiredis "yomi-boot/redis"
	"yomi-boot/shared/enums"
	"yomi-boot/snowflake"
	"yomi-boot/token"

	redisv9 "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// Host 插件 Init 阶段共享上下文：Doc 供各插件自取本段配置；其余为运行时产物。
// MustInitInfra 成功后映射为 bootstrap.Infra。
type Host struct {
	Doc *config.Document

	AppCtx appctx.AppCtx // 由 app 插件写入
	DbType enums.DbType  // 由 db 插件写入（db 未启用时为零值）

	DB                *gorm.DB
	RedisClient       *redisv9.Client
	RedisService      yomiredis.RedisService
	DistLockFactory   distlock.DistLockFactory
	RateLimiter       ratelimit.LimitService
	KafkaComponents   *yomikafka.Components
	KafkaPublisher    yomikafka.PublisherService
	KafkaConsumer     yomikafka.ConsumerService
	IDGenerator       snowflake.Generator
	TokenService      token.TokenService
	HttpRecordService httprecord.HttpRecordService
	TxExecutor        db.TxExecutor
	ConsulClient      *consul.Client
	ConsulServiceID   string
	DtmServer         string
	EventComponents   *event.Components
}
