// Package appctx 提供应用基础元信息与进程时区配置
//
// @author zwei
package appctx

import (
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/boot/config"
	"gitee.com/hq_project/yomi-boot/boot/errors"
)

// SectionKey YAML 根 key
const SectionKey = "app"

// DefaultTimezone 未配置 app.timezone 时的默认时区
const DefaultTimezone = "UTC"

// Config 表示服务名称、环境与时区配置
type Config struct {
	Name     string `mapstructure:"name"`     // Name 是服务名称
	Env      string `mapstructure:"env"`      // Env 是当前运行环境
	Version  string `mapstructure:"version"`  // Version 是当前版本号
	Timezone string `mapstructure:"timezone"` // Timezone 是进程时区，启动时锁定到 time.Local，默认 UTC
}

// Normalize 填充安全默认值
func (c *Config) Normalize() {
	if c == nil {
		return
	}
	c.Timezone = strings.TrimSpace(c.Timezone)
	if c.Timezone == "" || strings.EqualFold(c.Timezone, DefaultTimezone) {
		c.Timezone = DefaultTimezone
	}
}

// Validate 校验应用基础配置语义
func (c Config) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("app.name 不能为空")
	}
	tz := strings.TrimSpace(c.Timezone)
	if tz == "" {
		return errors.New("app.timezone 不能为空")
	}
	if strings.EqualFold(tz, "Local") {
		return errors.New("app.timezone 不支持 Local，请使用 IANA 时区名（如 UTC、Asia/Shanghai）")
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return errors.Wrap(err, "app.timezone 无效: %s", tz)
	}
	return nil
}

// ConfigSection 返回 app 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &Config{}, func(c *Config, _ *config.Document) error {
		c.Normalize()
		return c.Validate()
	})
}
