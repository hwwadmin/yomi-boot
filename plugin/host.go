// Package plugin 初始化阶段共享上下文
//
// @author zwei
package plugin

import (
	"yt-boot/appctx"
	"yt-boot/config"
	"yt-boot/consul"
	"yt-boot/db"
	"yt-boot/distlock"
	"yt-boot/event"
	"yt-boot/httprecord"
	ytkafka "yt-boot/kafka"
	"yt-boot/ratelimit"
	ytredis "yt-boot/redis"
	"yt-boot/shared/enums"
	"yt-boot/snowflake"
	"yt-boot/token"

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
	RedisService      ytredis.RedisService
	DistLockFactory   distlock.DistLockFactory
	RateLimiter       ratelimit.LimitService
	KafkaComponents   *ytkafka.Components
	KafkaPublisher    ytkafka.PublisherService
	KafkaConsumer     ytkafka.ConsumerService
	IDGenerator       snowflake.Generator
	TokenService      token.TokenService
	HttpRecordService httprecord.HttpRecordService
	TxExecutor        db.TxExecutor
	ConsulClient      *consul.Client
	ConsulServiceID   string
	DtmServer         string
	EventComponents   *event.Components
}
