// Package token 提供令牌基础配置
//
// @author zwei
package token

import (
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// Config 表示令牌基础配置
type Config struct {
	Enabled           bool     // Enabled 是否启用令牌服务
	TokenTTLMinutes   int      // TokenTTLMinutes 是令牌有效期分钟数
	AllowMultiLogin   bool     // AllowMultiLogin 表示是否允许并发登录
	StorePrefix       string   // StorePrefix 是令牌存储前缀标识
	SkipValidatePaths []string // SkipValidatePaths 是无需执行令牌校验的接口地址列表，按请求路径填写
}

// Validate 校验令牌配置本段语义（仅 enabled=true 时校验）
func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.TokenTTLMinutes <= 0 {
		return errors.New("token_ttl_minutes 必须大于 0")
	}
	if strings.TrimSpace(c.StorePrefix) == "" {
		return errors.New("store_prefix 不能为空")
	}
	for _, skipPath := range c.SkipValidatePaths {
		if skipPath != strings.TrimSpace(skipPath) {
			return errors.New("skip_validate_paths 不能含首尾空格")
		}
	}
	return nil
}

// TokenTTL 返回令牌有效期
func (c Config) TokenTTL() time.Duration {
	return time.Duration(c.TokenTTLMinutes) * time.Minute
}
