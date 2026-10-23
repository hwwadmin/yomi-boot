package router

import (
	"fmt"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

const (
	defaultMaxRequestBodyBytes      int64 = 1 << 20 // 1MB
	defaultShutdownTimeoutSeconds         = 10
	defaultReadTimeoutSeconds             = 10
	defaultWriteTimeoutSeconds            = 15
	defaultReadHeaderTimeoutSeconds       = 5
	defaultIdleTimeoutSeconds             = 90
	requestTimeoutBuffer                  = 2 * time.Second
)

// Config 表示 HTTP 服务运行时配置
type Config struct {
	Port                     int      // Port 是 HTTP 监听端口号
	ReadTimeoutSeconds       int      // ReadTimeoutSeconds 是读超时秒数，未配置默认 10
	WriteTimeoutSeconds      int      // WriteTimeoutSeconds 是写超时秒数，未配置默认 15
	ReadHeaderTimeoutSeconds int      // ReadHeaderTimeoutSeconds 是读请求头超时秒数，未配置默认 5
	IdleTimeoutSeconds       int      // IdleTimeoutSeconds 是空闲连接超时秒数，未配置默认 90
	ShutdownTimeoutSeconds   int      // ShutdownTimeoutSeconds 是停机超时秒数，未配置默认 10
	MaxRequestBodyBytes      int64    // MaxRequestBodyBytes 是请求体最大字节数，未配置或 <=0 时默认 1MB
	RouterPrefix             string   // RouterPrefix 是路由组前缀，如 /admin/api 或 /app/api
	TrustedProxies           []string // TrustedProxies 是可信反向代理 IP/CIDR，空则仅信任 RemoteAddr
}

// Validate 校验 HTTP 配置本段语义
func (c Config) Validate() error {
	if c.Port <= 0 {
		return errors.New("http.port 必须大于 0")
	}
	if c.MaxRequestBodyBytes < 0 {
		return errors.New("http.max_request_body_bytes 不能小于 0")
	}
	if c.ReadTimeoutSeconds < 0 {
		return errors.New("http.read_timeout_seconds 不能为负数")
	}
	if c.WriteTimeoutSeconds < 0 {
		return errors.New("http.write_timeout_seconds 不能为负数")
	}
	if c.ReadHeaderTimeoutSeconds < 0 {
		return errors.New("http.read_header_timeout_seconds 不能为负数")
	}
	if c.IdleTimeoutSeconds < 0 {
		return errors.New("http.idle_timeout_seconds 不能为负数")
	}
	if c.ShutdownTimeoutSeconds < 0 {
		return errors.New("http.shutdown_timeout_seconds 不能为负数")
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

// ReadTimeout 返回 HTTP 读超时，未配置时默认 10 秒
func (c Config) ReadTimeout() time.Duration {
	seconds := c.ReadTimeoutSeconds
	if seconds <= 0 {
		seconds = defaultReadTimeoutSeconds
	}
	return time.Duration(seconds) * time.Second
}

// WriteTimeout 返回 HTTP 写超时，未配置时默认 15 秒
func (c Config) WriteTimeout() time.Duration {
	seconds := c.WriteTimeoutSeconds
	if seconds <= 0 {
		seconds = defaultWriteTimeoutSeconds
	}
	return time.Duration(seconds) * time.Second
}

// ReadHeaderTimeout 返回读请求头超时，未配置时默认 5 秒
func (c Config) ReadHeaderTimeout() time.Duration {
	seconds := c.ReadHeaderTimeoutSeconds
	if seconds <= 0 {
		seconds = defaultReadHeaderTimeoutSeconds
	}
	return time.Duration(seconds) * time.Second
}

// IdleTimeout 返回空闲连接超时，未配置时默认 90 秒
func (c Config) IdleTimeout() time.Duration {
	seconds := c.IdleTimeoutSeconds
	if seconds <= 0 {
		seconds = defaultIdleTimeoutSeconds
	}
	return time.Duration(seconds) * time.Second
}

// ShutdownTimeout 返回优雅停机超时，未配置时默认 10 秒
func (c Config) ShutdownTimeout() time.Duration {
	seconds := c.ShutdownTimeoutSeconds
	if seconds <= 0 {
		seconds = defaultShutdownTimeoutSeconds
	}
	return time.Duration(seconds) * time.Second
}

// MaxRequestBodyBytesOrDefault 返回请求体最大字节数，未配置或 <=0 时默认 1MB
func (c Config) MaxRequestBodyBytesOrDefault() int64 {
	if c.MaxRequestBodyBytes <= 0 {
		return defaultMaxRequestBodyBytes
	}
	return c.MaxRequestBodyBytes
}

// RequestTimeout 返回请求级业务处理超时，取 write_timeout（未配置默认 15s）减 2s 作为响应收尾缓冲；不足时返回 0 表示不启用
func (c Config) RequestTimeout() time.Duration {
	timeout := c.WriteTimeout() - requestTimeoutBuffer
	if timeout <= 0 {
		return 0
	}
	return timeout
}
