// Package plugin 初始化阶段共享上下文
//
// @author zwei
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
	"gitee.com/hq_project/yomi-boot/core/config"
	"gitee.com/hq_project/yomi-boot/core/shared/enums"

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
	RateLimiter           redisratelimit.LimitService
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
	DtmClient             *dtm.Client
	EventComponents       *event.Components
}
