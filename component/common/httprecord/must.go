// Package httprecord Fail Fast 消费端点
//
// @author zwei
package httprecord

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/common/redisstream"
	"gitee.com/hq_project/yomi-boot/component/core/logger"

	yomikafka "gitee.com/hq_project/yomi-boot/component/common/kafka"
)

// MustStartKafkaRecordConsumer 启动 HTTP 访问记录 Kafka 消费者，失败时 log + panic
func MustStartKafkaRecordConsumer(
	parentCtx context.Context,
	consumer yomikafka.ConsumerService,
	id string,
	cfg KafkaRecordConsumerConfig,
	handler RecordHandler,
) {
	if err := StartKafkaRecordConsumer(parentCtx, consumer, id, cfg, handler); err != nil {
		logger.ErrorWithoutCtx("HTTP 访问记录 Kafka 消费者启动失败", "error", err)
		panic(err)
	}
}

// MustStartMemoryRecordConsumer 启动 HTTP 访问记录内存队列消费者，失败时 log + panic
func MustStartMemoryRecordConsumer(
	parentCtx context.Context,
	svc *MemoryHttpRecordService,
	cfg MemoryRecordConsumerConfig,
	handler RecordHandler,
) {
	if err := StartMemoryRecordConsumer(parentCtx, svc, cfg, handler); err != nil {
		logger.ErrorWithoutCtx("HTTP 访问记录内存消费者启动失败", "error", err)
		panic(err)
	}
}

// MustStartRedisStreamRecordConsumer 启动 HTTP 访问记录 Redis Stream 消费者，失败时 log + panic
func MustStartRedisStreamRecordConsumer(
	parentCtx context.Context,
	consumer redisstream.ConsumerService,
	id string,
	cfg RedisStreamRecordConsumerConfig,
	handler RecordHandler,
) {
	if err := StartRedisStreamRecordConsumer(parentCtx, consumer, id, cfg, handler); err != nil {
		logger.ErrorWithoutCtx("HTTP 访问记录 Redis Stream 消费者启动失败", "error", err)
		panic(err)
	}
}
