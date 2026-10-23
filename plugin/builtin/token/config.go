package tokenplugin

import (
	"gitee.com/hq_project/yomi-boot/component/common/token"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// SectionKey YAML 根 key
const SectionKey = "token"

// BootConfig 表示 token 配置段的 YAML 绑定
type BootConfig struct {
	Enabled           *bool    `mapstructure:"enabled"`             // Enabled 是否启用令牌服务，未配置默认 false
	TokenTTLMinutes   int      `mapstructure:"token_ttl_minutes"`   // TokenTTLMinutes 是令牌有效期分钟数
	AllowMultiLogin   bool     `mapstructure:"allow_multi_login"`   // AllowMultiLogin 表示是否允许并发登录
	StorePrefix       string   `mapstructure:"store_prefix"`        // StorePrefix 是令牌存储前缀标识
	SkipValidatePaths []string `mapstructure:"skip_validate_paths"` // SkipValidatePaths 是无需执行令牌校验的接口地址列表，按请求路径填写
}

// EnabledOrDefault 返回是否启用令牌服务，未配置默认 false
func (c BootConfig) EnabledOrDefault() bool {
	return c.Enabled != nil && *c.Enabled
}

// ToConfig 转换为运行时配置
func (c BootConfig) ToConfig() token.Config {
	return token.Config{
		MiddlewareEnabled: c.EnabledOrDefault(),
		TokenTTLMinutes:   c.TokenTTLMinutes,
		AllowMultiLogin:   c.AllowMultiLogin,
		StorePrefix:       c.StorePrefix,
		SkipValidatePaths: c.SkipValidatePaths,
	}
}

// Validate 校验 token 配置段语义（仅 enabled=true 时校验）
func (c BootConfig) Validate() error {
	if !c.EnabledOrDefault() {
		return nil
	}
	if err := c.ToConfig().Validate(); err != nil {
		return errors.Wrap(err, "%s", SectionKey)
	}
	return nil
}

// ConfigSection 返回 token 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &BootConfig{}, func(c *BootConfig, _ *config.Config) error {
		return c.Validate()
	})
}
