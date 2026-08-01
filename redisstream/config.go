// Package redisstream 提供基于 Redis Stream 的消息发布与消费组封装。
//
// 本包独立于 kafka 与 RedisService：仅依赖 go-redis 的 *redis.Client，
// 由业务按需启用，不作为统一消息门面。
//
// @author zwei
package redisstream

import (
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/config"
	"gitee.com/hq_project/yomi-boot/errors"
)

// SectionKey YAML 根 key
const SectionKey = "redis_stream"

// 默认运行参数
const (
	defaultBlockMS         = 2000
	defaultCount           = 16
	defaultClaimMinIdleMS  = 60000
	defaultClaimIntervalMS = 30000
	defaultMaxLen          = 100000
)

// Config 表示 Redis Stream 能力配置
type Config struct {
	Enabled           *bool  `mapstructure:"enabled"`             // Enabled 是否启用，未配置默认 false
	KeyPrefix         string `mapstructure:"key_prefix"`          // KeyPrefix 拼在 stream 名之前
	DefaultMaxLen     *int64 `mapstructure:"default_maxlen"`      // DefaultMaxLen 发布时默认近似裁剪长度；nil 用默认，0 表示不裁剪
	BlockMS           int    `mapstructure:"block_ms"`            // BlockMS 是 XREADGROUP 阻塞毫秒，0 表示使用默认值
	Count             int64  `mapstructure:"count"`               // Count 是单次拉取条数，0 表示使用默认值
	ClaimMinIdleMS    int    `mapstructure:"claim_min_idle_ms"`   // ClaimMinIdleMS 是 PEL 认领最小空闲毫秒，0 表示使用默认值
	ClaimIntervalMS   int    `mapstructure:"claim_interval_ms"`   // ClaimIntervalMS 是 PEL 回收周期毫秒，0 表示使用默认值
	MaxHandlerRetries *int   `mapstructure:"max_handler_retries"` // MaxHandlerRetries 是 handler 返回 Retry 时的最大重试次数；nil 默认 -1（无限）
}

// EnabledOrDefault 返回是否启用，未配置默认 false
func (c Config) EnabledOrDefault() bool {
	return config.BoolOrDefault(c.Enabled)
}

// Validate 校验配置语义（仅 enabled=true 时校验）
func (c Config) Validate() error {
	if !c.EnabledOrDefault() {
		return nil
	}
	if c.DefaultMaxLen != nil && *c.DefaultMaxLen < 0 {
		return errors.New("redis_stream.default_maxlen 不能小于 0")
	}
	if c.BlockMS < 0 {
		return errors.New("redis_stream.block_ms 不能小于 0")
	}
	if c.Count < 0 {
		return errors.New("redis_stream.count 不能小于 0")
	}
	if c.ClaimMinIdleMS < 0 {
		return errors.New("redis_stream.claim_min_idle_ms 不能小于 0")
	}
	if c.ClaimIntervalMS < 0 {
		return errors.New("redis_stream.claim_interval_ms 不能小于 0")
	}
	if c.MaxHandlerRetries != nil && *c.MaxHandlerRetries < -1 {
		return errors.New("redis_stream.max_handler_retries 不能小于 -1")
	}
	return nil
}

// KeyPrefixOrDefault 返回规范化后的 key 前缀
func (c Config) KeyPrefixOrDefault() string {
	return strings.TrimSpace(c.KeyPrefix)
}

// DefaultMaxLenOrDefault 返回发布默认裁剪长度；nil 用包默认，显式 0 表示不裁剪
func (c Config) DefaultMaxLenOrDefault() int64 {
	if c.DefaultMaxLen == nil {
		return defaultMaxLen
	}
	return *c.DefaultMaxLen
}

// Block 返回 XREADGROUP 阻塞时长
func (c Config) Block() time.Duration {
	ms := c.BlockMS
	if ms <= 0 {
		ms = defaultBlockMS
	}
	return time.Duration(ms) * time.Millisecond
}

// CountOrDefault 返回单次拉取条数
func (c Config) CountOrDefault() int64 {
	if c.Count <= 0 {
		return defaultCount
	}
	return c.Count
}

// ClaimMinIdle 返回 PEL 认领最小空闲时长
func (c Config) ClaimMinIdle() time.Duration {
	ms := c.ClaimMinIdleMS
	if ms <= 0 {
		ms = defaultClaimMinIdleMS
	}
	return time.Duration(ms) * time.Millisecond
}

// ClaimInterval 返回 PEL 回收周期
func (c Config) ClaimInterval() time.Duration {
	ms := c.ClaimIntervalMS
	if ms <= 0 {
		ms = defaultClaimIntervalMS
	}
	return time.Duration(ms) * time.Millisecond
}

// MaxHandlerRetriesOrDefault 返回 handler 最大重试次数，默认 -1（无限）
func (c Config) MaxHandlerRetriesOrDefault() int {
	if c.MaxHandlerRetries == nil {
		return -1
	}
	return *c.MaxHandlerRetries
}

// ConfigSection 返回 redis_stream 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &Config{}, func(c *Config, _ *config.Document) error {
		return c.Validate()
	})
}
