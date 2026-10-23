package routerplugin

import (
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/router"
)

// SectionKey YAML 根 key
const SectionKey = "http"

// BootConfig 表示 http 配置段的 YAML 绑定。
// HTTP 是进程本身，不提供 enabled；无 HTTP 进程属于独立的 worker-only 模式。
type BootConfig struct {
	Port                     int      `mapstructure:"port"`                        // Port 是 HTTP 监听端口号
	ReadTimeoutSeconds       int      `mapstructure:"read_timeout_seconds"`        // ReadTimeoutSeconds 是读超时秒数，未配置默认 10
	WriteTimeoutSeconds      int      `mapstructure:"write_timeout_seconds"`       // WriteTimeoutSeconds 是写超时秒数，未配置默认 15
	ReadHeaderTimeoutSeconds int      `mapstructure:"read_header_timeout_seconds"` // ReadHeaderTimeoutSeconds 是读请求头超时秒数，未配置默认 5
	IdleTimeoutSeconds       int      `mapstructure:"idle_timeout_seconds"`        // IdleTimeoutSeconds 是空闲连接超时秒数，未配置默认 90
	ShutdownTimeoutSeconds   int      `mapstructure:"shutdown_timeout_seconds"`    // ShutdownTimeoutSeconds 是停机超时秒数，未配置默认 10
	MaxRequestBodyBytes      int64    `mapstructure:"max_request_body_bytes"`      // MaxRequestBodyBytes 是请求体最大字节数，未配置或 <=0 时默认 1MB
	RouterPrefix             string   `mapstructure:"router_prefix"`               // RouterPrefix 是路由组前缀，如 /admin/api 或 /app/api
	TrustedProxies           []string `mapstructure:"trusted_proxies"`             // TrustedProxies 是可信反向代理 IP/CIDR，空则仅信任 RemoteAddr
}

// ToConfig 转换为运行时配置；字段与 router.Config 完全一致，新增字段须同步两者。
func (c BootConfig) ToConfig() router.Config {
	return router.Config(c)
}

// Validate 校验 http 配置段语义
func (c BootConfig) Validate() error {
	return c.ToConfig().Validate()
}

// ConfigSection 返回 http 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &BootConfig{}, func(c *BootConfig, _ *config.Config) error {
		return c.Validate()
	})
}
