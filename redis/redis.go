// Package redis 提供 Redis 客户端连接能力
//
// @author zwei
package redis

import (
	"time"
	"yomi-boot/appctx"
	"yomi-boot/errors"
	"yomi-boot/logger"

	"github.com/redis/go-redis/v9"
)

// NewClient 根据 Redis 配置创建并验证客户端连接；调用方应确保 cfg 已通过 Validate
func NewClient(cfg Config) (*redis.Client, error) {
	return newClient(cfg)
}

// newClient 假定 cfg 已通过语义校验，仅处理连接与 Ping 验证
func newClient(cfg Config) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	ctx, cancel := appctx.BackgroundWithTimeout(3 * time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, errors.Wrap(err, "redis连接验证失败")
	}

	logger.InfoWithoutCtx("初始化redis链接成功", "addr", cfg.Addr, "db", cfg.DB)

	return client, nil
}
