package redisratelimitplugin

import (
	"gitee.com/hq_project/yomi-boot/component/common/redisratelimit"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// SectionKey YAML 根 key
const SectionKey = "ratelimit"

// BootConfig 表示 ratelimit 配置段的 YAML 绑定
type BootConfig struct {
	Enabled          *bool  `mapstructure:"enabled"`            // Enabled 是否启用限流组件，未配置默认 false
	KeyPrefix        string `mapstructure:"key_prefix"`         // KeyPrefix 是 Redis 限流 Key 前缀
	RedisErrorPolicy string `mapstructure:"redis_error_policy"` // RedisErrorPolicy 是 Redis 故障策略：fail_open / fail_closed
}

// EnabledOrDefault 返回是否启用限流，未配置默认 false
func (c BootConfig) EnabledOrDefault() bool {
	return c.Enabled != nil && *c.Enabled
}

// ToConfig 转换为运行时配置
func (c BootConfig) ToConfig() redisratelimit.Config {
	return redisratelimit.Config{
		KeyPrefix:        c.KeyPrefix,
		RedisErrorPolicy: c.RedisErrorPolicy,
	}
}

// Validate 校验 ratelimit 配置段语义（仅 enabled=true 时校验）
func (c BootConfig) Validate() error {
	if !c.EnabledOrDefault() {
		return nil
	}
	if err := c.ToConfig().Validate(); err != nil {
		return errors.Wrap(err, "%s", SectionKey)
	}
	return nil
}

// ConfigSection 返回 ratelimit 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &BootConfig{}, func(c *BootConfig, _ *config.Config) error {
		return c.Validate()
	})
}
