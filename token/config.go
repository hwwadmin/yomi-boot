// Package token 提供令牌基础配置
//
// @author zwei
package token

import (
	"strings"
	"time"
	"yomi-boot/config"
	"yomi-boot/errors"
)

// SectionKey YAML 根 key
const SectionKey = "auth"

// Config 表示令牌基础配置
type Config struct {
	Enabled           *bool    `mapstructure:"enabled"`             // Enabled 是否启用令牌服务，未配置默认 false
	TokenTTLMinutes   int      `mapstructure:"token_ttl_minutes"`   // TokenTTLMinutes 是令牌有效期分钟数
	AllowMultiLogin   bool     `mapstructure:"allow_multi_login"`   // AllowMultiLogin 表示是否允许并发登录
	StorePrefix       string   `mapstructure:"store_prefix"`        // StorePrefix 是令牌存储前缀标识
	SkipValidatePaths []string `mapstructure:"skip_validate_paths"` // SkipValidatePaths 是无需执行令牌校验的接口地址列表，按请求路径填写
}

// EnabledOrDefault 返回是否启用令牌服务，未配置默认 false
func (c Config) EnabledOrDefault() bool {
	return config.BoolOrDefault(c.Enabled)
}

// Validate 校验令牌配置本段语义（仅 enabled=true 时校验）
func (c Config) Validate() error {
	if !c.EnabledOrDefault() {
		return nil
	}
	if c.TokenTTLMinutes <= 0 {
		return errors.New("auth.token_ttl_minutes 必须大于 0")
	}
	if strings.TrimSpace(c.StorePrefix) == "" {
		return errors.New("auth.store_prefix 不能为空")
	}
	for _, skipPath := range c.SkipValidatePaths {
		if skipPath != strings.TrimSpace(skipPath) {
			return errors.New("auth.skip_validate_paths 不能含首尾空格")
		}
	}
	return nil
}

// TokenTTL 返回令牌有效期
func (c Config) TokenTTL() time.Duration {
	return time.Duration(c.TokenTTLMinutes) * time.Minute
}

// ConfigSection 返回 auth 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &Config{}, func(c *Config, _ *config.Document) error {
		return c.Validate()
	})
}
