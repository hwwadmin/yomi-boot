package consulplugin

import (
	"gitee.com/hq_project/yomi-boot/component/cloud/consul"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// SectionKey YAML 根 key
const SectionKey = "consul"

// BootConfig 表示 consul 配置段的 YAML 绑定
type BootConfig struct {
	Enabled               *bool  `mapstructure:"enabled"`                 // Enabled 是否启用 Consul 注册发现，未配置默认 false
	Addr                  string `mapstructure:"addr"`                    // Addr 是 Consul 服务地址
	ServiceName           string `mapstructure:"service_name"`            // ServiceName 是注册到 Consul 的服务名称
	ServiceID             string `mapstructure:"service_id"`              // ServiceID 是注册到 Consul 的服务实例 ID
	ServiceAddress        string `mapstructure:"service_address"`         // ServiceAddress 是注册到 Consul 的服务地址（host:port）
	CheckAddress          string `mapstructure:"check_address"`           // CheckAddress 是健康检查地址
	DiscoveryCacheSeconds int    `mapstructure:"discovery_cache_seconds"` // DiscoveryCacheSeconds 是服务发现缓存秒数
	DiscoveryURLScheme    string `mapstructure:"discovery_url_scheme"`    // DiscoveryURLScheme 服务发现 URL 协议前缀，如 http:// / https://，留空则不拼接
	HTTPTimeoutSeconds    int    `mapstructure:"http_timeout_seconds"`    // HTTPTimeoutSeconds 是访问 Consul Agent 的 HTTP 超时秒数，未配置默认 10
	// DeregisterEnabled 是否在健康检查连续失败后自动注销，未配置默认 true
	DeregisterEnabled *bool `mapstructure:"deregister_enabled"`
	// DeregisterAfterSeconds 是健康检查连续失败后的注销秒数，未配置默认 1800（30 分钟），最小 60
	DeregisterAfterSeconds int `mapstructure:"deregister_after_seconds"`
}

// EnabledOrDefault 返回是否启用 Consul，未配置默认 false
func (c BootConfig) EnabledOrDefault() bool {
	return c.Enabled != nil && *c.Enabled
}

// ToConfig 转换为运行时配置
func (c BootConfig) ToConfig() consul.Config {
	return consul.Config{
		Addr:                   c.Addr,
		ServiceName:            c.ServiceName,
		ServiceID:              c.ServiceID,
		ServiceAddress:         c.ServiceAddress,
		CheckAddress:           c.CheckAddress,
		DiscoveryCacheSeconds:  c.DiscoveryCacheSeconds,
		DiscoveryURLScheme:     c.DiscoveryURLScheme,
		HTTPTimeoutSeconds:     c.HTTPTimeoutSeconds,
		DeregisterEnabled:      c.DeregisterEnabled,
		DeregisterAfterSeconds: c.DeregisterAfterSeconds,
	}
}

// Validate 校验 consul 配置段语义（仅 enabled=true 时校验）
func (c BootConfig) Validate() error {
	if !c.EnabledOrDefault() {
		return nil
	}
	if err := c.ToConfig().Validate(); err != nil {
		return errors.Wrap(err, "%s", SectionKey)
	}
	return nil
}

// ConfigSection 返回 consul 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &BootConfig{}, func(c *BootConfig, _ *config.Config) error {
		return c.Validate()
	})
}
