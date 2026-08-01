// Package redisstream 组件装配
//
// @author zwei
package redisstream

import (
	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/logger"

	redisv9 "github.com/redis/go-redis/v9"
)

// Components 聚合 Redis Stream 发布者与消费者管理器
type Components struct {
	Publisher PublisherService // Publisher 是消息发布者
	Consumer  ConsumerService  // Consumer 是消费者管理器
}

// NewComponents 基于已有 Redis 客户端创建组件；不关闭 client
func NewComponents(cfg Config, client *redisv9.Client) (*Components, error) {
	if client == nil {
		return nil, errors.New("redisstream: Redis client 不能为 nil")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	runtime := toRuntimeConfig(cfg)
	logger.InfoWithoutCtx(
		"初始化 Redis Stream 组件成功",
		"key_prefix", runtime.KeyPrefix,
		"default_maxlen", runtime.DefaultMaxLen,
		"block_ms", runtime.Block.Milliseconds(),
		"count", runtime.Count,
	)

	return &Components{
		Publisher: NewPublisher(client, runtime),
		Consumer:  NewConsumerManager(client, runtime),
	}, nil
}

// Close 停止消费者并关闭发布者；不关闭底层 Redis client
func (c *Components) Close() error {
	if c == nil {
		return nil
	}
	var joined error
	if c.Consumer != nil {
		joined = errors.Join(joined, c.Consumer.Close())
	}
	if c.Publisher != nil {
		joined = errors.Join(joined, c.Publisher.Close())
	}
	return joined
}

// toRuntimeConfig 将配置转换为运行时默认参数
func toRuntimeConfig(cfg Config) RuntimeConfig {
	return RuntimeConfig{
		KeyPrefix:     cfg.KeyPrefixOrDefault(),
		DefaultMaxLen: cfg.DefaultMaxLenOrDefault(),
		Block:         cfg.Block(),
		Count:         cfg.CountOrDefault(),
		ClaimMinIdle:  cfg.ClaimMinIdle(),
		ClaimInterval: cfg.ClaimInterval(),
		StopTimeout:   cfg.StopTimeout(),
	}
}
