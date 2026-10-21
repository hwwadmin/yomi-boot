// @author zwei

package rpc

import "time"

const (
	defaultRequestTimeout = 3 * time.Second // defaultRequestTimeout 是 InvokeRequest.Timeout 的默认值

	defaultMaxAttempts      = 3                      // defaultMaxAttempts 是默认最大尝试次数
	defaultBaseBackoff      = 50 * time.Millisecond  // defaultBaseBackoff 是首次重试退避
	defaultMaxBackoff       = 200 * time.Millisecond // defaultMaxBackoff 是退避上限
	defaultHTTPTimeout      = 30 * time.Second       // defaultHTTPTimeout 是单次 HTTP 尝试的后备超时
	defaultMaxResponseBytes = 4 << 20                // defaultMaxResponseBytes 是默认响应体上限（4 MiB）

	defaultBreakerWindowSize    = 100             // defaultBreakerWindowSize 是默认熔断窗口大小
	defaultBreakerMinRequests   = 20              // defaultBreakerMinRequests 是默认熔断最小请求数
	defaultBreakerFailureRate   = 0.5             // defaultBreakerFailureRate 是默认失败率阈值
	defaultBreakerSlowRate      = 0.5             // defaultBreakerSlowRate 是默认慢调用率阈值
	defaultBreakerSlowThreshold = 2 * time.Second // defaultBreakerSlowThreshold 是默认慢调用阈值
	defaultBreakerOpenDuration  = 5 * time.Second // defaultBreakerOpenDuration 是默认熔断打开时长
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

// httpClientOptions HTTP RPC 客户端内部参数
// 只保留客户端级配置；单次调用的超时、次数、响应上限由 InvokeRequest 控制
type httpClientOptions struct {
	HTTPTimeout time.Duration  // HTTPTimeout 是单次 HTTP 尝试的后备超时，默认 30s
	BaseBackoff time.Duration  // BaseBackoff 是首次重试退避，默认 50ms
	MaxBackoff  time.Duration  // MaxBackoff 是退避上限，默认 200ms
	Breaker     BreakerOptions // Breaker 是服务级熔断参数
}

// ClientOption 修改 HTTP RPC 客户端参数
type ClientOption func(*httpClientOptions)

// WithHTTPTimeout 设置单次 HTTP 尝试的后备超时
func WithHTTPTimeout(d time.Duration) ClientOption {
	return func(o *httpClientOptions) {
		if d > 0 {
			o.HTTPTimeout = d
		}
	}
}

// WithBackoff 设置重试退避基数与上限
func WithBackoff(base, max time.Duration) ClientOption {
	return func(o *httpClientOptions) {
		if base > 0 {
			o.BaseBackoff = base
		}
		if max > 0 {
			o.MaxBackoff = max
		}
	}
}

// WithBreaker 设置熔断器参数
// 熔断器按 serviceName 共享；同一服务只有第一次创建时的配置生效
func WithBreaker(cfg BreakerOptions) ClientOption {
	return func(o *httpClientOptions) {
		o.Breaker = cfg
	}
}

// defaultHTTPClientOptions 返回 HTTP RPC 客户端默认参数
func defaultHTTPClientOptions() httpClientOptions {
	return httpClientOptions{
		HTTPTimeout: defaultHTTPTimeout,
		BaseBackoff: defaultBaseBackoff,
		MaxBackoff:  defaultMaxBackoff,
		Breaker:     defaultBreakerOptions(),
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

// applyHTTPClientOptions 合并选项并归一化参数
func applyHTTPClientOptions(opts []ClientOption) httpClientOptions {
	cfg := defaultHTTPClientOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return normalizeHTTPClientOptions(cfg)
}

// normalizeHTTPClientOptions 填充非法或缺失的客户端参数
func normalizeHTTPClientOptions(cfg httpClientOptions) httpClientOptions {
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = defaultHTTPTimeout
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
