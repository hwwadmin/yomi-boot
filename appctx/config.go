// Package appctx 提供应用基础元信息配置
//
// @author zwei
package appctx

import (
	"strings"
	"yomi-boot/config"
	"yomi-boot/errors"
)

// SectionKey YAML 根 key
const SectionKey = "app"

// Config 表示服务名称、环境和版本配置
type Config struct {
	Name    string `mapstructure:"name"`    // Name 是服务名称
	Env     string `mapstructure:"env"`     // Env 是当前运行环境
	Version string `mapstructure:"version"` // Version 是当前版本号
}

// Validate 校验应用基础配置语义
func (c Config) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("app.name 不能为空")
	}
	return nil
}

// ConfigSection 返回 app 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &Config{}, func(c *Config, _ *config.Document) error {
		return c.Validate()
	})
}
