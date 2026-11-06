// Package redisratelimit 提供限流配置
//
// @author zwei
package redisratelimit

import (
	"strings"

	"github.com/hwwadmin/yomi-boot/component/core/errors"
)

const (
	defaultRateLimitKeyPrefix = "yomi:rl"
	maxRateLimitKeyPrefixLen  = 64
)

// Config 表示调用式限流配置（不包含 Redis 连接；连接沿用 redis 段）
// Redis 故障策略不在这里，由每次调用的 Rule 决定。
type Config struct {
	KeyPrefix string // KeyPrefix 是 Redis 限流 Key 前缀
}

// KeyPrefixOrDefault 返回限流 Key 前缀
func (c Config) KeyPrefixOrDefault() string {
	prefix := strings.TrimSpace(c.KeyPrefix)
	if prefix == "" {
		return defaultRateLimitKeyPrefix
	}
	return prefix
}

// Validate 校验限流配置语义
func (c Config) Validate() error {
	return validateRateLimitKeyPrefix(c.KeyPrefixOrDefault())
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
