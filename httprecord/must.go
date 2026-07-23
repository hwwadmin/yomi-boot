// Package httprecord Fail Fast 消费端点
//
// @author zwei
package httprecord

import (
	"context"
	"yomi-boot/logger"

	yomikafka "yomi-boot/kafka"
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
