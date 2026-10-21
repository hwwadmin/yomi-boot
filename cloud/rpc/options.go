// @author zwei

package rpc

import "time"

const (
	defaultMaxAttempts      = 3
	defaultBaseBackoff      = 50 * time.Millisecond
	defaultMaxBackoff       = 200 * time.Millisecond
	defaultHTTPTimeout      = 30 * time.Second
	defaultMaxResponseBytes = 4 << 20 // 4 MiB

	defaultBreakerWindowSize    = 100
	defaultBreakerMinRequests   = 20
	defaultBreakerFailureRate   = 0.5
	defaultBreakerSlowRate      = 0.5
	defaultBreakerSlowThreshold = 2 * time.Second
	defaultBreakerOpenDuration  = 5 * time.Second
)

// BreakerOptions 熔断器参数
// 失败率或慢调用率达到阈值且窗口内请求数达到 MinRequests 时打开熔断
type BreakerOptions struct {
	WindowSize    int           // WindowSize 是滑动窗口保留的尝试数，默认 100
	MinRequests   int           // MinRequests 是窗口内达到该请求数后才评估，默认 20
	FailureRate   float64       // FailureRate 是失败率阈值，默认 0.5
	SlowRate      float64       // SlowRate 是慢调用率阈值，默认 0.5
	SlowThreshold time.Duration // SlowThreshold 是慢调用耗时阈值，默认 2s
	OpenDuration  time.Duration // OpenDuration 是熔断打开时长，默认 5s
}

// clientOptions RPC 客户端内部参数
type clientOptions struct {
	HTTPTimeout      time.Duration
	MaxAttempts      int
	BaseBackoff      time.Duration
	MaxBackoff       time.Duration
	MaxResponseBytes int64
	Breaker          BreakerOptions
}

// ClientOption 修改 RPC 客户端参数
type ClientOption func(*clientOptions)

// WithHTTPTimeout 设置单次 HTTP 请求超时
func WithHTTPTimeout(d time.Duration) ClientOption {
	return func(o *clientOptions) {
		if d > 0 {
			o.HTTPTimeout = d
		}
	}
}

// WithMaxAttempts 设置最大尝试次数（含首次）
func WithMaxAttempts(n int) ClientOption {
	return func(o *clientOptions) {
		if n > 0 {
			o.MaxAttempts = n
		}
	}
}

// WithBackoff 设置重试退避基数与上限
func WithBackoff(base, max time.Duration) ClientOption {
	return func(o *clientOptions) {
		if base > 0 {
			o.BaseBackoff = base
		}
		if max > 0 {
			o.MaxBackoff = max
		}
	}
}

// WithMaxResponseBytes 设置响应体最大字节数
func WithMaxResponseBytes(n int64) ClientOption {
	return func(o *clientOptions) {
		if n > 0 {
			o.MaxResponseBytes = n
		}
	}
}

// WithBreaker 设置熔断器参数
func WithBreaker(cfg BreakerOptions) ClientOption {
	return func(o *clientOptions) {
		o.Breaker = cfg
	}
}

// defaultClientOptions 返回客户端默认参数
func defaultClientOptions() clientOptions {
	return clientOptions{
		HTTPTimeout:      defaultHTTPTimeout,
		MaxAttempts:      defaultMaxAttempts,
		BaseBackoff:      defaultBaseBackoff,
		MaxBackoff:       defaultMaxBackoff,
		MaxResponseBytes: defaultMaxResponseBytes,
		Breaker:          defaultBreakerOptions(),
	}
}

// defaultBreakerOptions 返回熔断器默认参数
func defaultBreakerOptions() BreakerOptions {
	return BreakerOptions{
		WindowSize:    defaultBreakerWindowSize,
		MinRequests:   defaultBreakerMinRequests,
		FailureRate:   defaultBreakerFailureRate,
		SlowRate:      defaultBreakerSlowRate,
		SlowThreshold: defaultBreakerSlowThreshold,
		OpenDuration:  defaultBreakerOpenDuration,
	}
}

// applyClientOptions 合并选项并归一化参数
func applyClientOptions(opts []ClientOption) clientOptions {
	cfg := defaultClientOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return normalizeClientOptions(cfg)
}

// normalizeClientOptions 填充非法或缺失的客户端参数
func normalizeClientOptions(cfg clientOptions) clientOptions {
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = defaultHTTPTimeout
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = defaultMaxAttempts
	}
	if cfg.BaseBackoff <= 0 {
		cfg.BaseBackoff = defaultBaseBackoff
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = defaultMaxBackoff
	}
	if cfg.MaxBackoff < cfg.BaseBackoff {
		cfg.MaxBackoff = cfg.BaseBackoff
	}
	if cfg.MaxResponseBytes <= 0 {
		cfg.MaxResponseBytes = defaultMaxResponseBytes
	}
	cfg.Breaker = normalizeBreakerOptions(cfg.Breaker)
	return cfg
}

// normalizeBreakerOptions 填充非法或缺失的熔断参数
func normalizeBreakerOptions(cfg BreakerOptions) BreakerOptions {
	if cfg.WindowSize <= 0 {
		cfg.WindowSize = defaultBreakerWindowSize
	}
	if cfg.MinRequests <= 0 {
		cfg.MinRequests = defaultBreakerMinRequests
	}
	if cfg.MinRequests > cfg.WindowSize {
		cfg.WindowSize = cfg.MinRequests
	}
	if cfg.FailureRate <= 0 || cfg.FailureRate > 1 {
		cfg.FailureRate = defaultBreakerFailureRate
	}
	if cfg.SlowRate <= 0 || cfg.SlowRate > 1 {
		cfg.SlowRate = defaultBreakerSlowRate
	}
	if cfg.SlowThreshold <= 0 {
		cfg.SlowThreshold = defaultBreakerSlowThreshold
	}
	if cfg.OpenDuration <= 0 {
		cfg.OpenDuration = defaultBreakerOpenDuration
	}
	return cfg
}
