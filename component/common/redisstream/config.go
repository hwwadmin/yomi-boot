// Package redisstream 提供基于 Redis Stream 的消息发布与消费组封装。
//
// 本包独立于 kafka 与 RedisService：仅依赖 go-redis 的 *redis.Client，
// 由业务按需启用，不作为统一消息门面。
//
// 投递语义为至少一次（at-least-once），不保证恰好一次：HandleRetry 未 XACK 的消息
// 会留在 PEL，空闲超过 claim_min_idle 后可被同组其它实例 XAUTOCLAIM 再次投递，
// 从而出现并发或重复处理。业务 Handler 必须自行保证幂等。
//
// 消费契约：每条投递只调用一次 Handler；是否当场重试或阻塞由业务在 Handler 内自行决定。
// HandleRetry 表示不 XACK、留在 PEL 并继续拉取后续消息，再投递依赖 XAUTOCLAIM。
//
// @author zwei
package redisstream

import (
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/logger"
	"gitee.com/hq_project/yomi-boot/core/config"
	"gitee.com/hq_project/yomi-boot/core/errors"
)

// SectionKey YAML 根 key
const SectionKey = "redis_stream"

// 默认运行参数
const (
	defaultBlockMS            = 2000
	defaultCount              = 16
	defaultClaimMinIdleMS     = 60000
	defaultClaimIntervalMS    = 30000
	defaultMaxLen             = 100000
	defaultStopTimeoutMS      = 30000
	defaultPublishWorkerNum   = 4
	defaultPublishWorkerSize  = 256
	defaultPublishSendTimeout = 3 * time.Second
)

// PublishWorkerConfig 表示 Redis Stream 异步发布专用工作池配置
type PublishWorkerConfig struct {
	WorkNum              int `mapstructure:"work_num"`               // WorkNum 是并发 worker 数，0 表示使用默认值
	Size                 int `mapstructure:"size"`                   // Size 是任务队列容量，0 表示使用默认值
	SubmitTimeoutSeconds int `mapstructure:"submit_timeout_seconds"` // SubmitTimeoutSeconds 是入队等待超时秒数，0 表示使用 worker 默认值
}

// Config 表示 Redis Stream 能力配置
type Config struct {
	Enabled         *bool               `mapstructure:"enabled"`           // Enabled 是否启用，未配置默认 false
	KeyPrefix       string              `mapstructure:"key_prefix"`        // KeyPrefix 拼在逻辑 stream 名之前（直接拼接）；非空时建议以 ':' 结尾，如 "rs:" → "rs:orders"
	DefaultMaxLen   *int64              `mapstructure:"default_maxlen"`    // DefaultMaxLen 发布时默认近似裁剪长度；nil 用默认，0 表示不裁剪
	BlockMS         int                 `mapstructure:"block_ms"`          // BlockMS 是 XREADGROUP 阻塞毫秒，0 表示使用默认值
	Count           int64               `mapstructure:"count"`             // Count 是单次拉取条数，0 表示使用默认值
	ClaimMinIdleMS  int                 `mapstructure:"claim_min_idle_ms"` // ClaimMinIdleMS 是 PEL 认领最小空闲毫秒，0 表示使用默认值；过小会加大多实例重复投递概率
	ClaimIntervalMS int                 `mapstructure:"claim_interval_ms"` // ClaimIntervalMS 是 PEL 回收周期毫秒，0 表示使用默认值
	StopTimeoutMS   int                 `mapstructure:"stop_timeout_ms"`   // StopTimeoutMS 是 Stop/Close 等待消费协程退出的超时毫秒，0 表示使用默认值
	PublishWorker   PublishWorkerConfig `mapstructure:"publish_worker"`    // PublishWorker 是异步发布工作池配置
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
	if c.StopTimeoutMS < 0 {
		return errors.New("redis_stream.stop_timeout_ms 不能小于 0")
	}
	if err := c.PublishWorker.Validate(); err != nil {
		return err
	}
	prefix := strings.TrimSpace(c.KeyPrefix)
	if prefix != "" && !strings.HasSuffix(prefix, ":") {
		logger.WarnWithoutCtx(
			"redis_stream.key_prefix 建议以 ':' 结尾，否则会与 stream 名直接粘连",
			"key_prefix", prefix,
		)
	}
	return nil
}

// Validate 校验 Redis Stream 异步发布工作池配置
func (c PublishWorkerConfig) Validate() error {
	if c.WorkNum < 0 {
		return errors.New("redis_stream.publish_worker.work_num 不能小于 0")
	}
	if c.Size < 0 {
		return errors.New("redis_stream.publish_worker.size 不能小于 0")
	}
	if c.SubmitTimeoutSeconds < 0 {
		return errors.New("redis_stream.publish_worker.submit_timeout_seconds 不能小于 0")
	}
	return nil
}

// KeyPrefixOrDefault 返回规范化后的 key 前缀（不做自动补 ':'，由配置自行带分隔符）
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

// StopTimeout 返回 Stop/Close 等待消费协程退出的超时
func (c Config) StopTimeout() time.Duration {
	ms := c.StopTimeoutMS
	if ms <= 0 {
		ms = defaultStopTimeoutMS
	}
	return time.Duration(ms) * time.Millisecond
}

// ConfigSection 返回 redis_stream 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &Config{}, func(c *Config, _ *config.Document) error {
		return c.Validate()
	})
}
