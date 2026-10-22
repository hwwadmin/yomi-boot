package redisstreamplugin

import (
	"gitee.com/hq_project/yomi-boot/component/common/redisstream"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// SectionKey YAML 根 key
const SectionKey = "redis_stream"

// PublishWorkerBootConfig 表示 Redis Stream 异步发布工作池的 YAML 绑定
type PublishWorkerBootConfig struct {
	WorkNum              int `mapstructure:"work_num"`               // WorkNum 是并发 worker 数，0 表示使用默认值
	Size                 int `mapstructure:"size"`                   // Size 是任务队列容量，0 表示使用默认值
	SubmitTimeoutSeconds int `mapstructure:"submit_timeout_seconds"` // SubmitTimeoutSeconds 是入队等待超时秒数，0 表示使用 worker 默认值
}

// BootConfig 表示 redis_stream 配置段的 YAML 绑定
type BootConfig struct {
	Enabled         *bool                   `mapstructure:"enabled"`           // Enabled 是否启用，未配置默认 false
	KeyPrefix       string                  `mapstructure:"key_prefix"`        // KeyPrefix 拼在逻辑 stream 名之前（直接拼接）；非空时建议以 ':' 结尾，如 "rs:" → "rs:orders"
	DefaultMaxLen   *int64                  `mapstructure:"default_maxlen"`    // DefaultMaxLen 发布时默认近似裁剪长度；nil 用默认，0 表示不裁剪
	BlockMS         int                     `mapstructure:"block_ms"`          // BlockMS 是 XREADGROUP 阻塞毫秒，0 表示使用默认值
	Count           int64                   `mapstructure:"count"`             // Count 是单次拉取条数，0 表示使用默认值
	ClaimMinIdleMS  int                     `mapstructure:"claim_min_idle_ms"` // ClaimMinIdleMS 是 PEL 认领最小空闲毫秒，0 表示使用默认值；过小会加大多实例重复投递概率
	ClaimIntervalMS int                     `mapstructure:"claim_interval_ms"` // ClaimIntervalMS 是 PEL 回收周期毫秒，0 表示使用默认值
	StopTimeoutMS   int                     `mapstructure:"stop_timeout_ms"`   // StopTimeoutMS 是 Stop/Close 等待消费协程退出的超时毫秒，0 表示使用默认值
	PublishWorker   PublishWorkerBootConfig `mapstructure:"publish_worker"`    // PublishWorker 是异步发布工作池配置
}

// EnabledOrDefault 返回是否启用，未配置默认 false
func (c BootConfig) EnabledOrDefault() bool {
	return c.Enabled != nil && *c.Enabled
}

// ToConfig 转换为运行时配置
func (c BootConfig) ToConfig() redisstream.Config {
	return redisstream.Config{
		KeyPrefix:       c.KeyPrefix,
		DefaultMaxLen:   c.DefaultMaxLen,
		BlockMS:         c.BlockMS,
		Count:           c.Count,
		ClaimMinIdleMS:  c.ClaimMinIdleMS,
		ClaimIntervalMS: c.ClaimIntervalMS,
		StopTimeoutMS:   c.StopTimeoutMS,
		PublishWorker: redisstream.PublishWorkerConfig{
			WorkNum:              c.PublishWorker.WorkNum,
			Size:                 c.PublishWorker.Size,
			SubmitTimeoutSeconds: c.PublishWorker.SubmitTimeoutSeconds,
		},
	}
}

// Validate 校验 redis_stream 配置段语义（仅 enabled=true 时校验）
func (c BootConfig) Validate() error {
	if !c.EnabledOrDefault() {
		return nil
	}
	if err := c.ToConfig().Validate(); err != nil {
		return errors.Wrap(err, "%s", SectionKey)
	}
	return nil
}

// ConfigSection 返回 redis_stream 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &BootConfig{}, func(c *BootConfig, _ *config.Config) error {
		return c.Validate()
	})
}
