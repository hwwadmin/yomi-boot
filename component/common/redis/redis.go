// Package redis 提供 Redis 客户端连接能力
//
// @author zwei
package redis

import (
	"context"
	"strings"

	"github.com/hwwadmin/yomi-boot/component/core/appctx"
	"github.com/hwwadmin/yomi-boot/component/core/errors"
	"github.com/hwwadmin/yomi-boot/component/core/logger"

	"github.com/redis/go-redis/v9"
)

// NewClient 根据 Redis 配置创建并验证客户端连接；ctx 约束连接验证，调用方应确保 cfg 已通过 Validate
func NewClient(ctx context.Context, cfg Config) (*redis.Client, error) {
	return newClient(ctx, cfg)
}

// newClient 建立连接并 Ping。地址去掉两端空白；空地址、负数库编号在拨号前返回错误。
// 命令失败不自动重试：驱动把 MaxRetries 0 当成再试 3 次，-1 才是关闭。
// 非幂等脚本在响应丢失后若被再执行，限流会先扣配额再返回拒绝，验证码会先删掉再报不存在。
func newClient(ctx context.Context, cfg Config) (*redis.Client, error) {
	addr := strings.TrimSpace(cfg.Addr)
	if addr == "" {
		return nil, errors.New("addr 不能为空")
	}
	if cfg.DB < 0 {
		return nil, errors.New("db 不能小于 0")
	}

	timeout := cfg.CommandTimeout()
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: cfg.Password,
		DB:       cfg.DB,
		// -1 表示不再自动重试。填 0 会被驱动改成再试 3 次。
		MaxRetries: -1,
		// 单条指令的最长等待时间同时约束 socket 读写；阻塞命令由 go-redis 按 Block 自行放宽
		ReadTimeout:  timeout,
		WriteTimeout: timeout,
		// go-redis 默认忽略 context deadline，开启后 RedisHelper 的指令超时与调用方 deadline 才生效
		ContextTimeoutEnabled: true,
	})

	pingCtx, cancel := appctx.WithTimeout(ctx, timeout)
	defer cancel()

	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, errors.Wrap(err, "redis连接验证失败")
	}

	logger.InfoWithoutCtx("初始化redis链接成功", "addr", addr, "db", cfg.DB)

	return client, nil
}
