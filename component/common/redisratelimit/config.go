// Package redisratelimit 提供限流配置
//
// @author zwei
package redisratelimit

import (
	"strings"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

const (
	// RedisErrorPolicyFailOpen Redis 故障时放行
	RedisErrorPolicyFailOpen = "fail_open"
	// RedisErrorPolicyFailClosed Redis 故障时拒绝
	RedisErrorPolicyFailClosed = "fail_closed"

	defaultRateLimitKeyPrefix        = "yomi:rl"
	defaultRateLimitRedisErrorPolicy = RedisErrorPolicyFailOpen
	maxRateLimitKeyPrefixLen         = 64
)

// Config 表示调用式限流配置（不包含 Redis 连接；连接沿用 redis 段）
type Config struct {
	KeyPrefix        string // KeyPrefix 是 Redis 限流 Key 前缀
	RedisErrorPolicy string // RedisErrorPolicy 是 Redis 故障策略：fail_open / fail_closed
}

// KeyPrefixOrDefault 返回限流 Key 前缀
func (c Config) KeyPrefixOrDefault() string {
	prefix := strings.TrimSpace(c.KeyPrefix)
	if prefix == "" {
		return defaultRateLimitKeyPrefix
	}
	return prefix
}

// RedisErrorPolicyOrDefault 返回 Redis 故障策略
func (c Config) RedisErrorPolicyOrDefault() string {
	policy := strings.TrimSpace(strings.ToLower(c.RedisErrorPolicy))
	if policy == "" {
		return defaultRateLimitRedisErrorPolicy
	}
	return policy
}

// Validate 校验限流配置语义
func (c Config) Validate() error {
	prefix := c.KeyPrefixOrDefault()
	if err := validateRateLimitKeyPrefix(prefix); err != nil {
		return err
	}
	policy := c.RedisErrorPolicyOrDefault()
	if policy != RedisErrorPolicyFailOpen && policy != RedisErrorPolicyFailClosed {
		return errors.New("redis_error_policy 仅支持 fail_open 或 fail_closed")
	}
	return nil
}

// validateRateLimitKeyPrefix 校验限流 Key 前缀格式
func validateRateLimitKeyPrefix(prefix string) error {
	if prefix == "" {
		return errors.New("key_prefix 不能为空")
	}
	if len(prefix) > maxRateLimitKeyPrefixLen {
		return errors.Newf("key_prefix 长度不能超过 %d", maxRateLimitKeyPrefixLen)
	}
	for _, r := range prefix {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ':' || r == '_' {
			continue
		}
		return errors.New("key_prefix 仅允许小写字母、数字、冒号和下划线")
	}
	return nil
}
