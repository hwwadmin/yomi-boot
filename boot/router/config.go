// Package router 提供 HTTP 服务配置
//
// @author zwei
package router

import (
	"fmt"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/boot/config"
	"gitee.com/hq_project/yomi-boot/boot/errors"
)

const (
	// SectionKey YAML 根 key
	SectionKey = "http"

	defaultMaxRequestBodyBytes int64 = 1 << 20 // 1MB
	requestTimeoutBuffer             = 2 * time.Second
)

// Config 表示 HTTP 服务配置
type Config struct {
	Port                   int      `mapstructure:"port"`                     // Port 是 HTTP 监听端口号
	ReadTimeoutSeconds     int      `mapstructure:"read_timeout_seconds"`     // ReadTimeoutSeconds 是读超时秒数
	WriteTimeoutSeconds    int      `mapstructure:"write_timeout_seconds"`    // WriteTimeoutSeconds 是写超时秒数
	ShutdownTimeoutSeconds int      `mapstructure:"shutdown_timeout_seconds"` // ShutdownTimeoutSeconds 是停机超时秒数
	MaxRequestBodyBytes    int64    `mapstructure:"max_request_body_bytes"`   // MaxRequestBodyBytes 是请求体最大字节数，未配置或 <=0 时默认 1MB
	RouterPrefix           string   `mapstructure:"router_prefix"`            // RouterPrefix 是路由组前缀，如 /admin/api 或 /app/api
	TrustedProxies         []string `mapstructure:"trusted_proxies"`          // TrustedProxies 是可信反向代理 IP/CIDR，空则仅信任 RemoteAddr
}

// Validate 校验 HTTP 配置本段语义
func (c Config) Validate() error {
	if c.Port <= 0 {
		return errors.New("http.port 必须大于 0")
	}
	if c.MaxRequestBodyBytes < 0 {
		return errors.New("http.max_request_body_bytes 不能小于 0")
	}
	if c.RouterPrefix != strings.TrimSpace(c.RouterPrefix) {
		return errors.New("http.router_prefix 不能含首尾空格")
	}
	return nil
}

// RouterPrefixClean 返回去首尾空格后的路由前缀
func (c Config) RouterPrefixClean() string {
	return strings.TrimSpace(c.RouterPrefix)
}

// ListenAddr 返回 http.Server 所需的监听地址
func (c Config) ListenAddr() string {
	return fmt.Sprintf(":%d", c.Port)
}

// ReadTimeout 返回 HTTP 读超时
func (c Config) ReadTimeout() time.Duration {
	return time.Duration(c.ReadTimeoutSeconds) * time.Second
}

// WriteTimeout 返回 HTTP 写超时
func (c Config) WriteTimeout() time.Duration {
	return time.Duration(c.WriteTimeoutSeconds) * time.Second
}

// ShutdownTimeout 返回优雅停机超时
func (c Config) ShutdownTimeout() time.Duration {
	return time.Duration(c.ShutdownTimeoutSeconds) * time.Second
}

// MaxRequestBodyBytesOrDefault 返回请求体最大字节数，未配置或 <=0 时默认 1MB
func (c Config) MaxRequestBodyBytesOrDefault() int64 {
	if c.MaxRequestBodyBytes <= 0 {
		return defaultMaxRequestBodyBytes
	}
	return c.MaxRequestBodyBytes
}

// RequestTimeout 返回请求级业务处理超时，取 write_timeout 减 2s 作为响应收尾缓冲；不足时返回 0 表示不启用
func (c Config) RequestTimeout() time.Duration {
	timeout := c.WriteTimeout() - requestTimeoutBuffer
	if timeout <= 0 {
		return 0
	}
	return timeout
}

// ConfigSection 返回 http 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &Config{}, func(c *Config, _ *config.Document) error {
		return c.Validate()
	})
}
