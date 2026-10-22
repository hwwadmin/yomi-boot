package cors

import (
	"slices"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// Config 表示跨域资源共享运行时配置
type Config struct {
	MiddlewareEnabled *bool    // MiddlewareEnabled 控制 CORS 中间件是否执行；nil 视为启用；由 BootConfig.enabled 映射
	AllowOrigins      []string // AllowOrigins 是允许的来源地址列表，* 表示允许所有来源
	AllowMethods      []string // AllowMethods 是允许的 HTTP 方法列表
	AllowHeaders      []string // AllowHeaders 是允许的请求头列表
	MaxAge            int      // MaxAge 是预检请求缓存秒数（Access-Control-Max-Age），0 表示默认 86400
	AllowCredentials  bool     // AllowCredentials 是否允许携带凭证（Cookie 等）
}

// Validate 校验 CORS 配置语义
func (c Config) Validate() error {
	if c.MaxAge < 0 {
		return errors.New("cors.max_age 不能为负数")
	}
	if len(c.AllowOrigins) > 1 && slices.Contains(c.AllowOrigins, "*") {
		return errors.New(`cors.allow_origins 为 "*" 时不能同时配置其他来源`)
	}
	return nil
}

// MiddlewareEnabledOrDefault 返回 CORS 中间件是否执行，未配置默认 true
func (c Config) MiddlewareEnabledOrDefault() bool {
	return c.MiddlewareEnabled == nil || *c.MiddlewareEnabled
}
