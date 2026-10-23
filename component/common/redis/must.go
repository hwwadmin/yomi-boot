// Package redis Fail Fast 初始化
//
// @author zwei
package redis

import (
	"gitee.com/hq_project/yomi-boot/component/core/logger"

	"github.com/redis/go-redis/v9"
)

// MustNewClient 校验配置并创建 Redis 客户端，失败时 log + panic
func MustNewClient(cfg Config) *redis.Client {
	if err := cfg.Validate(); err != nil {
		logger.ErrorWithoutCtx("redis 配置校验失败", "error", err)
		panic(err)
	}
	client, err := newClient(cfg)
	if err != nil {
		logger.ErrorWithoutCtx("redis 初始化失败", "error", err)
		panic(err)
	}
	return client
}
