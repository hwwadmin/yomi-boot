package corsplugin

import (
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/cors"
)

// SectionKey YAML 根 key
const SectionKey = "cors"

// BootConfig 表示 cors 配置段的 YAML 绑定
type BootConfig struct {
	Enabled          *bool    `mapstructure:"enabled"`           // Enabled 是否启用 CORS，未配置默认 true
	AllowOrigins     []string `mapstructure:"allow_origins"`     // AllowOrigins 是允许的来源地址列表，* 表示允许所有来源
	AllowMethods     []string `mapstructure:"allow_methods"`     // AllowMethods 是允许的 HTTP 方法列表
	AllowHeaders     []string `mapstructure:"allow_headers"`     // AllowHeaders 是允许的请求头列表
	MaxAge           int      `mapstructure:"max_age"`           // MaxAge 是预检请求缓存秒数（Access-Control-Max-Age），0 表示默认 86400
	AllowCredentials bool     `mapstructure:"allow_credentials"` // AllowCredentials 是否允许携带凭证（Cookie 等）
}

// EnabledOrDefault 返回是否启用 CORS，未配置默认 true
func (c BootConfig) EnabledOrDefault() bool {
	return config.BoolOr(c.Enabled, true)
}

// ToConfig 转换为运行时配置
func (c BootConfig) ToConfig() cors.Config {
	return cors.Config{
		MiddlewareEnabled: c.Enabled,
		AllowOrigins:      c.AllowOrigins,
		AllowMethods:      c.AllowMethods,
		AllowHeaders:      c.AllowHeaders,
		MaxAge:            c.MaxAge,
		AllowCredentials:  c.AllowCredentials,
	}
}

// Validate 校验 cors 配置段语义（仅 enabled=true 时校验）
func (c BootConfig) Validate() error {
	if !c.EnabledOrDefault() {
		return nil
	}
	return c.ToConfig().Validate()
}

// ConfigSection 返回 cors 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &BootConfig{}, func(c *BootConfig, _ *config.Config) error {
		return c.Validate()
	})
}
