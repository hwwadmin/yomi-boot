// Package redis 提供 Redis 访问配置
//
// @author zwei
package redis

import (
	"strings"
	"yt-boot/config"
	"yt-boot/errors"
)

// SectionKey YAML 根 key
const SectionKey = "redis"

// Config 表示 Redis 访问配置
type Config struct {
	Enabled  *bool  `mapstructure:"enabled"`  // Enabled 是否启用 Redis，未配置默认 false
	Addr     string `mapstructure:"addr"`     // Addr 是 Redis 地址
	Password string `mapstructure:"password"` // Password 是 Redis 密码
	DB       int    `mapstructure:"db"`       // DB 是 Redis 数据库编号
}

// EnabledOrDefault 返回是否启用 Redis，未配置默认 false
func (c Config) EnabledOrDefault() bool {
	return config.BoolOrDefault(c.Enabled)
}

// Validate 校验 Redis 配置语义（仅 enabled=true 时校验）
func (c Config) Validate() error {
	if !c.EnabledOrDefault() {
		return nil
	}
	if strings.TrimSpace(c.Addr) == "" {
		return errors.New("redis.addr 不能为空")
	}
	return nil
}

// ConfigSection 返回 redis 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &Config{}, func(c *Config, _ *config.Document) error {
		return c.Validate()
	})
}
