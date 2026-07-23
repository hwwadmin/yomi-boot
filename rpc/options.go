// Package rpc 提供 HTTP RPC 客户端选项
//
// @author zwei
package rpc

import "time"

const (
	defaultMaxAttempts         = 3
	defaultBaseBackoff         = 50 * time.Millisecond
	defaultMaxBackoff          = 200 * time.Millisecond
	defaultHTTPTimeout         = 30 * time.Second
	defaultBreakerFailures     = 5
	defaultBreakerOpenDuration = 5 * time.Second
)

// ClientOptions HTTP RPC 客户端韧性参数（读路径轻量重试 + 熔断）
type ClientOptions struct {
	MaxAttempts         int           // MaxAttempts 最大尝试次数（含首次），默认 3
	BaseBackoff         time.Duration // BaseBackoff 首次重试退避，默认 50ms
	MaxBackoff          time.Duration // MaxBackoff 退避上限，默认 200ms
	HTTPTimeout         time.Duration // HTTPTimeout 单次 HTTP 客户端超时，默认 30s
	BreakerFailures     int           // BreakerFailures 连续传输失败阈值，默认 5
	BreakerOpenDuration time.Duration // BreakerOpenDuration 熔断打开时长，默认 5s
}

// ClientOption 可选配置函数
type ClientOption func(*ClientOptions)

// WithMaxAttempts 设置最大尝试次数（含首次）
func WithMaxAttempts(n int) ClientOption {
	return func(o *ClientOptions) {
		if n > 0 {
			o.MaxAttempts = n
		}
	}
}

// WithBackoff 设置退避基数与上限
func WithBackoff(base, max time.Duration) ClientOption {
	return func(o *ClientOptions) {
		if base > 0 {
			o.BaseBackoff = base
		}
		if max > 0 {
			o.MaxBackoff = max
		}
	}
}

// WithHTTPTimeout 设置单次 HTTP 超时
func WithHTTPTimeout(d time.Duration) ClientOption {
	return func(o *ClientOptions) {
		if d > 0 {
			o.HTTPTimeout = d
		}
	}
}

// WithBreaker 设置熔断连续失败阈值与打开时长
func WithBreaker(failures int, openFor time.Duration) ClientOption {
	return func(o *ClientOptions) {
		if failures > 0 {
			o.BreakerFailures = failures
		}
		if openFor > 0 {
			o.BreakerOpenDuration = openFor
		}
	}
}

// defaultClientOptions 返回客户端默认韧性参数
func defaultClientOptions() ClientOptions {
	return ClientOptions{
		MaxAttempts:         defaultMaxAttempts,
		BaseBackoff:         defaultBaseBackoff,
		MaxBackoff:          defaultMaxBackoff,
		HTTPTimeout:         defaultHTTPTimeout,
		BreakerFailures:     defaultBreakerFailures,
		BreakerOpenDuration: defaultBreakerOpenDuration,
	}
}

// applyClientOptions 合并选项并校正退避上限
func applyClientOptions(opts []ClientOption) ClientOptions {
	cfg := defaultClientOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.MaxBackoff < cfg.BaseBackoff {
		cfg.MaxBackoff = cfg.BaseBackoff
	}
	return cfg
}
