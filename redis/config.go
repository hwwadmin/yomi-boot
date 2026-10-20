// Package redis 提供 Redis 访问配置
//
// @author zwei
package redis

import (
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/config"
	"gitee.com/hq_project/yomi-boot/errors"
)

// SectionKey YAML 根 key
const SectionKey = "redis"

// DefaultCommandTimeoutMs 未配置 command_timeout_ms 时的默认指令执行最长等待时间（毫秒）
const DefaultCommandTimeoutMs = 3000

// Config 表示 Redis 访问配置
type Config struct {
	Enabled          *bool  `mapstructure:"enabled"`            // Enabled 是否启用 Redis，未配置默认 false
	Addr             string `mapstructure:"addr"`               // Addr 是 Redis 地址
	Password         string `mapstructure:"password"`           // Password 是 Redis 密码
	DB               int    `mapstructure:"db"`                 // DB 是 Redis 数据库编号
	CommandTimeoutMs int    `mapstructure:"command_timeout_ms"` // CommandTimeoutMs 是单条指令执行的最长等待时间(毫秒)，未配置或 <=0 时默认 3000
}

// EnabledOrDefault 返回是否启用 Redis，未配置默认 false
func (c Config) EnabledOrDefault() bool {
	return config.BoolOrDefault(c.Enabled)
}

// CommandTimeout 返回单条指令执行的最长等待时间；未配置或 <=0 时默认 3000ms
func (c Config) CommandTimeout() time.Duration {
	ms := c.CommandTimeoutMs
	if ms <= 0 {
		ms = DefaultCommandTimeoutMs
	}
	return time.Duration(ms) * time.Millisecond
}

// Validate 校验 Redis 配置语义（仅 enabled=true 时校验）
func (c Config) Validate() error {
	if !c.EnabledOrDefault() {
		return nil
	}
	if strings.TrimSpace(c.Addr) == "" {
		return errors.New("redis.addr 不能为空")
	}
	if c.CommandTimeoutMs < 0 {
		return errors.New("redis.command_timeout_ms 不能小于 0")
	}
	return nil
}

// ConfigSection 返回 redis 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &Config{}, func(c *Config, _ *config.Document) error {
		return c.Validate()
	})
}
