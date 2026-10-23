package kafka

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/core/logger"
)

// MustNewPublisher 创建 Kafka 生产者，失败时 log + panic
func MustNewPublisher(ctx context.Context, cfg PublisherConfig) PublisherService {
	publisher, err := NewPublisher(ctx, cfg)
	if err != nil {
		logger.ErrorWithoutCtx("kafka 生产者初始化失败", "error", err)
		panic(err)
	}
	return publisher
}

// MustNewConsumerManager 创建 Kafka 消费者管理器，失败时 log + panic
func MustNewConsumerManager(ctx context.Context, cfg ConsumerManagerConfig) ConsumerService {
	manager, err := NewConsumerManager(ctx, cfg)
	if err != nil {
		logger.ErrorWithoutCtx("kafka 消费者管理器初始化失败", "error", err)
		panic(err)
	}
	return manager
}

// MustNewAdmin 创建 Kafka 管理端，失败时 log + panic
func MustNewAdmin(ctx context.Context, cfg ConnectionConfig) AdminService {
	admin, err := NewAdmin(ctx, cfg)
	if err != nil {
		logger.ErrorWithoutCtx("kafka 管理端初始化失败", "error", err)
		panic(err)
	}
	return admin
}
