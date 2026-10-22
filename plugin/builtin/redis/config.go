package redisplugin

import (
	yomiredis "gitee.com/hq_project/yomi-boot/component/common/redis"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// SectionKey YAML 根 key
const SectionKey = "redis"

// BootConfig 表示 redis 配置段的 YAML 绑定
type BootConfig struct {
	Enabled          *bool  `mapstructure:"enabled"`            // Enabled 是否启用 Redis，未配置默认 false
	Addr             string `mapstructure:"addr"`               // Addr 是 Redis 地址
	Password         string `mapstructure:"password"`           // Password 是 Redis 密码
	DB               int    `mapstructure:"db"`                 // DB 是 Redis 数据库编号
	CommandTimeoutMs int    `mapstructure:"command_timeout_ms"` // CommandTimeoutMs 是单条指令执行的最长等待时间(毫秒)，未配置或 <=0 时默认 3000
}

// EnabledOrDefault 返回是否启用 Redis，未配置默认 false
func (c BootConfig) EnabledOrDefault() bool {
	return c.Enabled != nil && *c.Enabled
}

// ToConfig 转换为运行时配置
func (c BootConfig) ToConfig() yomiredis.Config {
	return yomiredis.Config{
		Addr:             c.Addr,
		Password:         c.Password,
		DB:               c.DB,
		CommandTimeoutMs: c.CommandTimeoutMs,
	}
}

// Validate 校验 redis 配置段语义（仅 enabled=true 时校验）
func (c BootConfig) Validate() error {
	if !c.EnabledOrDefault() {
		return nil
	}
	if err := c.ToConfig().Validate(); err != nil {
		return errors.Wrap(err, "%s", SectionKey)
	}
	return nil
}

// ConfigSection 返回 redis 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &BootConfig{}, func(c *BootConfig, _ *config.Document) error {
		return c.Validate()
	})
}
