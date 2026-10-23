package kafka

import (
	"gitee.com/hq_project/yomi-boot/component/core/logger"
)

// MustNewPublisher 创建 Kafka 生产者，失败时 log + panic
func MustNewPublisher(cfg PublisherConfig) PublisherService {
	publisher, err := NewPublisher(cfg)
	if err != nil {
		logger.ErrorWithoutCtx("kafka 生产者初始化失败", "error", err)
		panic(err)
	}
	return publisher
}

// MustNewConsumerManager 创建 Kafka 消费者管理器，失败时 log + panic
func MustNewConsumerManager(cfg ConsumerManagerConfig) ConsumerService {
	manager, err := NewConsumerManager(cfg)
	if err != nil {
		logger.ErrorWithoutCtx("kafka 消费者管理器初始化失败", "error", err)
		panic(err)
	}
	return manager
}

// MustNewAdmin 创建 Kafka 管理端，失败时 log + panic
func MustNewAdmin(cfg ConnectionConfig) AdminService {
	admin, err := NewAdmin(cfg)
	if err != nil {
		logger.ErrorWithoutCtx("kafka 管理端初始化失败", "error", err)
		panic(err)
	}
	return admin
}
