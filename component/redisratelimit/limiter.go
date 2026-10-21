// Package redisratelimit 提供调用式分布式限流能力
//
// 依赖现有 boot/redis.RedisService（OpsRateLimit4Allow），不新建 Redis 连接。
// 用法：在 usecase 入口显式调用 Allow/AllowN；超限返回 ErrRateLimited。
//
// @author zwei
package redisratelimit

import (
	"context"

	"gitee.com/hq_project/yomi-boot/boot/errors"
	"gitee.com/hq_project/yomi-boot/boot/logger"
	yomiredis "gitee.com/hq_project/yomi-boot/component/redis"
)

// ErrRateLimited 请求触发限流
var ErrRateLimited = errors.NewNoStack("RATE_LIMITED")

// LimitService 调用式限流门面
type LimitService interface {
	// Allow 尝试消耗 1 次配额；超限返回 ErrRateLimited
	Allow(ctx context.Context, key string, rule Rule) error
	// AllowN 尝试消耗 n 次配额；超限返回 ErrRateLimited
	AllowN(ctx context.Context, key string, n int64, rule Rule) error
}

// limiter 基于 Redis 的限流实现
type limiter struct {
	redis    yomiredis.RedisService
	prefix   string
	failOpen bool
}

// noopLimiter 未启用时的空实现，Allow/AllowN 恒成功
type noopLimiter struct{}

// New 创建限流服务；未启用时返回 Noop（Allow 恒成功）
func New(redisService yomiredis.RedisService, cfg Config) LimitService {
	if !cfg.EnabledOrDefault() {
		return noopLimiter{}
	}
	if redisService == nil {
		panic("redisratelimit: RedisService 不能为 nil")
	}
	return &limiter{
		redis:    redisService,
		prefix:   cfg.KeyPrefixOrDefault(),
		failOpen: cfg.RedisErrorPolicyOrDefault() == RedisErrorPolicyFailOpen,
	}
}

// Allow 实现 LimitService.Allow
func (l *limiter) Allow(ctx context.Context, key string, rule Rule) error {
	return l.AllowN(ctx, key, 1, rule)
}

// AllowN 实现 LimitService.AllowN
func (l *limiter) AllowN(ctx context.Context, key string, n int64, rule Rule) error {
	if l == nil || l.redis == nil {
		return errors.New("redisratelimit: LimitService 未初始化")
	}
	if n <= 0 {
		return errors.New("redisratelimit: n 必须大于 0")
	}
	if err := rule.Validate(); err != nil {
		return err
	}
	fullKey, err := buildFullKey(l.prefix, key)
	if err != nil {
		return err
	}

	allowed, err := l.redis.OpsRateLimit4Allow(ctx, fullKey, rule.Limit, n, rule.Window)
	if err != nil {
		if l.failOpen {
			logger.Warn(ctx, "redisratelimit redis 故障，按 fail_open 放行", "key", fullKey, "error", err)
			return nil
		}
		return errors.Wrap(err, "redisratelimit redis 故障")
	}
	if !allowed {
		return errors.Sentinel(ErrRateLimited)
	}
	return nil
}

// Allow 实现 LimitService.Allow（Noop）
func (noopLimiter) Allow(context.Context, string, Rule) error { return nil }

// AllowN 实现 LimitService.AllowN（Noop）
func (noopLimiter) AllowN(context.Context, string, int64, Rule) error { return nil }

// 编译期校验接口实现
var (
	_ LimitService = (*limiter)(nil)
	_ LimitService = noopLimiter{}
)
