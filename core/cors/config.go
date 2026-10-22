// Package cors 提供跨域资源共享配置
//
// @author zwei
package cors

import "gitee.com/hq_project/yomi-boot/core/config"

// SectionKey YAML 根 key
const SectionKey = "cors"

// Config 表示跨域资源共享配置
type Config struct {
	AllowOrigins     []string `mapstructure:"allow_origins"`     // AllowOrigins 是允许的来源地址列表，* 表示允许所有来源
	AllowMethods     []string `mapstructure:"allow_methods"`     // AllowMethods 是允许的 HTTP 方法列表
	AllowHeaders     []string `mapstructure:"allow_headers"`     // AllowHeaders 是允许的请求头列表
	MaxAge           int      `mapstructure:"max_age"`           // MaxAge 是预检请求缓存秒数（Access-Control-Max-Age），0 表示默认 86400
	AllowCredentials bool     `mapstructure:"allow_credentials"` // AllowCredentials 是否允许携带凭证（Cookie 等）
}

// ConfigSection 返回 cors 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &Config{}, nil)
}
