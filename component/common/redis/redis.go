// Package redis 提供 Redis 客户端连接能力
//
// @author zwei
package redis

import (
	"gitee.com/hq_project/yomi-boot/component/common/appctx"
	"gitee.com/hq_project/yomi-boot/component/common/logger"
	"gitee.com/hq_project/yomi-boot/core/errors"

	"github.com/redis/go-redis/v9"
)

// NewClient 根据 Redis 配置创建并验证客户端连接；调用方应确保 cfg 已通过 Validate
func NewClient(cfg Config) (*redis.Client, error) {
	return newClient(cfg)
}

// newClient 假定 cfg 已通过语义校验，仅处理连接与 Ping 验证
func newClient(cfg Config) (*redis.Client, error) {
	timeout := cfg.CommandTimeout()
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
		// 单条指令的最长等待时间同时约束 socket 读写；阻塞命令由 go-redis 按 Block 自行放宽
		ReadTimeout:  timeout,
		WriteTimeout: timeout,
		// go-redis 默认忽略 context deadline，开启后 RedisHelper 的指令超时与调用方 deadline 才生效
		ContextTimeoutEnabled: true,
	})

	ctx, cancel := appctx.BackgroundWithTimeout(timeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, errors.Wrap(err, "redis连接验证失败")
	}

	logger.InfoWithoutCtx("初始化redis链接成功", "addr", cfg.Addr, "db", cfg.DB)

	return client, nil
}
