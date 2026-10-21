// @author zwei

package rpc

import (
	"net/http"
	"time"
)

// 默认参数：请求超时、重试退避、响应上限、连接池与熔断
const (
	defaultRequestTimeout = 3 * time.Second // 单次调用默认超时

	defaultMaxAttempts      = 3                      // 默认最大尝试次数
	defaultBaseBackoff      = 50 * time.Millisecond  // 首次重试退避
	defaultMaxBackoff       = 200 * time.Millisecond // 退避上限
	defaultHTTPTimeout      = 30 * time.Second       // 单次 HTTP 尝试的后备超时
	defaultMaxResponseBytes = 4 << 20                // 响应体上限（4 MiB）

	defaultMaxIdleConns        = 200 // 空闲连接总数
	defaultMaxIdleConnsPerHost = 100 // 每 host 空闲连接数

	defaultBreakerWindowSize    = 100             // 熔断窗口大小
	defaultBreakerMinRequests   = 20              // 熔断最小样本数
	defaultBreakerFailureRate   = 0.5             // 失败率阈值
	defaultBreakerSlowRate      = 0.5             // 慢调用率阈值
	defaultBreakerSlowThreshold = 2 * time.Second // 慢调用阈值
	defaultBreakerOpenDuration  = 5 * time.Second // 熔断打开时长
)

// BreakerOptions 熔断器参数
// 失败率或慢调用率达到阈值且对应窗口样本数达到 MinRequests 时打开
// 窗口按尝试计数；失败率看计入失败率的样本，慢调用率看全部尝试
type BreakerOptions struct {
	WindowSize    int           // WindowSize 是滑动窗口保留的尝试数，默认 100
	MinRequests   int           // MinRequests 是窗口内样本数达到该值后评估（按尝试计），默认 20
	FailureRate   float64       // FailureRate 是失败率阈值，默认 0.5
	SlowRate      float64       // SlowRate 是慢调用率阈值，默认 0.5
	SlowThreshold time.Duration // SlowThreshold 是慢调用耗时阈值，默认 2s
	OpenDuration  time.Duration // OpenDuration 是熔断打开时长，默认 5s
}

// httpClientOptions HTTP RPC 客户端内部参数
// 保存客户端级配置；单次调用的超时、次数、响应上限由 InvokeRequest 控制
type httpClientOptions struct {
	HTTPTimeout   time.Duration                                      // 单次 HTTP 尝试的后备超时，默认 30s
	BaseBackoff   time.Duration                                      // 首次重试退避，默认 50ms
	MaxBackoff    time.Duration                                      // 退避上限，默认 200ms
	Transport     http.RoundTripper                                  // 自定义传输；nil 时使用默认共享传输
	CheckRedirect func(req *http.Request, via []*http.Request) error // 重定向策略；nil 时使用默认策略
	LogBodies     bool                                               // 是否打印请求/响应体（Info），默认关闭
	Breaker       BreakerOptions                                     // 当前 client 的熔断参数
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

// WithTransport 设置自定义传输，用于 mTLS、私有 CA、代理、连接池等场景
// 传 nil 时保持默认共享传输
func WithTransport(transport http.RoundTripper) ClientOption {
	return func(o *httpClientOptions) {
		if transport != nil {
			o.Transport = transport
		}
	}
}

// WithCheckRedirect 设置重定向策略；默认不跟随，3xx 作为 HTTPStatusError 返回
// 跟随重定向时传自定义函数，并自行限制重定向次数；nil 保持默认策略
func WithCheckRedirect(fn func(req *http.Request, via []*http.Request) error) ClientOption {
	return func(o *httpClientOptions) {
		if fn != nil {
			o.CheckRedirect = fn
		}
	}
}

// WithBreaker 设置当前 client 的熔断器参数
// 每个 client 使用独立熔断器
func WithBreaker(cfg BreakerOptions) ClientOption {
	return func(o *httpClientOptions) {
		o.Breaker = cfg
	}
}

// WithLogBodies 控制是否打印请求体和响应体，默认关闭
// 开启后按 Info 级别输出；建议仅在 dev 打开
// 请求/响应体可能含敏感信息，日志最多保留 4 KiB
func WithLogBodies(enabled bool) ClientOption {
	return func(o *httpClientOptions) {
		o.LogBodies = enabled
	}
}

// defaultCheckRedirect 关闭自动跟随；3xx 作为 HTTPStatusError 交给调用方，
// POST 方法和请求体保持不变
func defaultCheckRedirect(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

// defaultTransport 是 RPC 客户端共享的默认传输；调高空闲连接数，提高高并发下的连接复用
var defaultTransport = newDefaultTransport()

// newDefaultTransport 克隆 http.DefaultTransport 并调整连接池；全局传输为自定义实现时直接复用
func newDefaultTransport() http.RoundTripper {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok || base == nil {
		return http.DefaultTransport
	}
	transport := base.Clone()
	transport.MaxIdleConns = defaultMaxIdleConns
	transport.MaxIdleConnsPerHost = defaultMaxIdleConnsPerHost
	return transport
}

// defaultHTTPClientOptions 返回 HTTP RPC 客户端默认参数
func defaultHTTPClientOptions() httpClientOptions {
	return httpClientOptions{
		HTTPTimeout:   defaultHTTPTimeout,
		BaseBackoff:   defaultBaseBackoff,
		MaxBackoff:    defaultMaxBackoff,
		Transport:     defaultTransport,
		CheckRedirect: defaultCheckRedirect,
		Breaker:       defaultBreakerOptions(),
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
	if cfg.Transport == nil {
		cfg.Transport = defaultTransport
	}
	if cfg.CheckRedirect == nil {
		cfg.CheckRedirect = defaultCheckRedirect
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
