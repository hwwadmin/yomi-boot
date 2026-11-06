// Package token 提供令牌基础配置
//
// @author zwei
package token

import (
	"math"
	"strings"
	"time"

	"github.com/hwwadmin/yomi-boot/component/core/errors"
)

// maxTokenTTLMinutes 是换算成 time.Duration 后仍为正数的最大分钟数。
// 再大时 minutes * time.Minute 会溢出成负数或绕回一个很短的正数，存储层会把非正 TTL 写成不过期。
const maxTokenTTLMinutes = int64(math.MaxInt64) / int64(time.Minute)

// Config 表示令牌基础配置
type Config struct {
	MiddlewareEnabled bool     // MiddlewareEnabled 控制鉴权中间件是否执行；由 BootConfig.enabled 映射
	TokenTTLMinutes   int      // TokenTTLMinutes 是令牌有效期分钟数
	AllowMultiLogin   bool     // AllowMultiLogin 表示是否允许并发登录
	StorePrefix       string   // StorePrefix 是令牌存储前缀标识
	SkipValidatePaths []string // SkipValidatePaths 是无需执行令牌校验的接口地址列表，按请求路径填写；根路径只写 "/"
}

// Validate 校验令牌配置语义
func (c Config) Validate() error {
	if err := validateTokenTTLMinutes(c.TokenTTLMinutes); err != nil {
		return err
	}
	if strings.TrimSpace(c.StorePrefix) == "" {
		return errors.New("store_prefix 不能为空")
	}
	for _, skipPath := range c.SkipValidatePaths {
		if skipPath != strings.TrimSpace(skipPath) {
			return errors.New("skip_validate_paths 不能含首尾空格")
		}
		if skipPath == "" || skipPath == "." {
			return errors.New("skip_validate_paths 根路径只能写 /")
		}
	}
	return nil
}

// TokenTTL 返回令牌有效期。分钟数须先通过 validateTokenTTLMinutes，否则乘积可能溢出。
func (c Config) TokenTTL() time.Duration {
	return time.Duration(c.TokenTTLMinutes) * time.Minute
}

// validateTokenTTLMinutes 拒绝非正分钟数，以及乘成 time.Duration 后不再是正数的分钟数。
func validateTokenTTLMinutes(minutes int) error {
	if minutes <= 0 {
		return errors.New("token_ttl_minutes 必须大于 0")
	}
	if int64(minutes) > maxTokenTTLMinutes {
		return errors.New("token_ttl_minutes 过大")
	}
	return nil
}
