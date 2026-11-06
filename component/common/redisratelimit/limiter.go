// Package redisratelimit 提供调用式分布式限流能力
//
// 依赖 component/common/redis.RedisService（OpsRateLimit4Allow），不新建 Redis 连接。
// 用法：在 usecase 入口显式调用 Allow/AllowN；超限返回 ErrRateLimited。
// Redis 故障策略写在 Rule 上，空白默认 fail_closed。
//
// @author zwei
package redisratelimit

import (
	"context"

	yomiredis "github.com/hwwadmin/yomi-boot/component/common/redis"
	"github.com/hwwadmin/yomi-boot/component/core/errors"
	"github.com/hwwadmin/yomi-boot/component/core/logger"
)

// ErrRateLimited 请求触发限流
var ErrRateLimited = errors.NewNoStack("RATE_LIMITED")

// LimitService 调用式限流门面
type LimitService interface {
	// Allow 尝试消耗 1 次配额；超限返回 ErrRateLimited。
	// 调用方 ctx 已取消或已过截止时间时返回该错误。
	Allow(ctx context.Context, key string, rule Rule) error
	// AllowN 尝试消耗 n 次配额；超限返回 ErrRateLimited。
	// n 超过 redis 原语单次上限时返回错误，不按故障策略放行。
	// 调用方 ctx 已取消或已过截止时间时返回该错误，不按故障策略放行。
	AllowN(ctx context.Context, key string, n int64, rule Rule) error
}

// limiter 基于 Redis 的限流实现
type limiter struct {
	redis  yomiredis.RedisService
	prefix string
}

// noopLimiter 未启用时的空实现，Allow/AllowN 恒成功
type noopLimiter struct{}

// New 创建限流服务；RedisService 为 nil 或配置非法时 panic
func New(redisService yomiredis.RedisService, cfg Config) LimitService {
	if redisService == nil {
		panic("redisratelimit: RedisService 不能为 nil")
	}
	if err := cfg.Validate(); err != nil {
		panic("redisratelimit: " + err.Error())
	}
	return &limiter{
		redis:  redisService,
		prefix: cfg.KeyPrefixOrDefault(),
	}
}

// NewNoop 创建空限流服务，Allow/AllowN 恒成功
func NewNoop() LimitService {
	return noopLimiter{}
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
	if n > yomiredis.MaxRateLimitN {
		return errors.Newf("redisratelimit: n 超过单次上限 %d", yomiredis.MaxRateLimitN)
	}
	fullKey, err := buildFullKey(l.prefix, key)
	if err != nil {
		return err
	}
	if err := contextErr(ctx); err != nil {
		return err
	}

	allowed, err := l.redis.OpsRateLimit4Allow(ctx, fullKey, rule.Limit, n, rule.Window)
	if err != nil {
		if ctxErr := contextErr(ctx); ctxErr != nil {
			return ctxErr
		}
		if rule.RedisErrorPolicyOrDefault() == RedisErrorPolicyFailOpen {
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

// contextErr 返回调用方 ctx 已经结束的原因。nil ctx 没有调用方取消信号。
func contextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
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
