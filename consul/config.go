// Package consul 提供 Consul 注册与发现配置
//
// @author zwei
package consul

import (
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
	"yt-boot/config"
	"yt-boot/errors"
)

const (
	// defaultDiscoveryCacheSeconds 是服务发现缓存的默认秒数
	defaultDiscoveryCacheSeconds = 30
	// defaultHTTPTimeoutSeconds 是 Consul Agent HTTP 客户端默认超时秒数
	defaultHTTPTimeoutSeconds = 10
)

// SectionKey YAML 根 key
const SectionKey = "consul"

// Config 保存 Consul 注册与发现配置
type Config struct {
	Enabled               *bool  `mapstructure:"enabled"`                 // Enabled 是否启用 Consul 注册发现，未配置默认 false
	Addr                  string `mapstructure:"addr"`                    // Addr 是 Consul 服务地址
	ServiceName           string `mapstructure:"service_name"`            // ServiceName 是注册到 Consul 的服务名称
	ServiceID             string `mapstructure:"service_id"`              // ServiceID 是注册到 Consul 的服务实例 ID
	ServiceAddress        string `mapstructure:"service_address"`         // ServiceAddress 是注册到 Consul 的服务地址（host:port）
	CheckAddress          string `mapstructure:"check_address"`           // CheckAddress 是健康检查地址
	DiscoveryCacheSeconds int    `mapstructure:"discovery_cache_seconds"` // DiscoveryCacheSeconds 是服务发现缓存秒数
	DiscoveryURLScheme    string `mapstructure:"discovery_url_scheme"`    // DiscoveryURLScheme 服务发现 URL 协议前缀，如 http:// / https://，留空则不拼接
	HTTPTimeoutSeconds    int    `mapstructure:"http_timeout_seconds"`    // HTTPTimeoutSeconds 是访问 Consul Agent 的 HTTP 超时秒数，未配置默认 10
}

// Validate 在 Consul 启用时校验本段配置
func (c Config) Validate() error {
	if !c.EnabledOrDefault() {
		return nil
	}
	if strings.TrimSpace(c.Addr) == "" {
		return errors.New("consul.addr 不能为空")
	}
	if strings.TrimSpace(c.ServiceID) == "" {
		return errors.New("consul.service_id 不能为空")
	}
	if c.DiscoveryCacheSeconds < 0 {
		return errors.New("consul.discovery_cache_seconds 不能为负数")
	}
	if c.HTTPTimeoutSeconds < 0 {
		return errors.New("consul.http_timeout_seconds 不能为负数")
	}
	if err := validateConsulHostPort(c.ServiceAddress, "consul.service_address"); err != nil {
		return err
	}
	if err := validateConsulCheckURL(c.CheckAddress, "consul.check_address"); err != nil {
		return err
	}
	if err := validateDiscoveryURLScheme(c.DiscoveryURLScheme, "consul.discovery_url_scheme"); err != nil {
		return err
	}
	return nil
}

// EnabledOrDefault 返回是否启用 Consul，未配置默认 false
func (c Config) EnabledOrDefault() bool {
	return config.BoolOrDefault(c.Enabled)
}

// DiscoveryCacheDuration 返回服务发现缓存 TTL，未配置时默认 30 秒
func (c Config) DiscoveryCacheDuration() time.Duration {
	if c.DiscoveryCacheSeconds <= 0 {
		return defaultDiscoveryCacheSeconds * time.Second
	}
	return time.Duration(c.DiscoveryCacheSeconds) * time.Second
}

// HTTPTimeoutOrDefault 返回访问 Consul Agent 的 HTTP 超时，未配置或为 0 时默认 10 秒
func (c Config) HTTPTimeoutOrDefault() time.Duration {
	if c.HTTPTimeoutSeconds <= 0 {
		return defaultHTTPTimeoutSeconds * time.Second
	}
	return time.Duration(c.HTTPTimeoutSeconds) * time.Second
}

// ServiceIDClean 返回配置的 Consul 服务实例 ID
func (c Config) ServiceIDClean() string {
	return strings.TrimSpace(c.ServiceID)
}

// ServiceNameOrDefault 返回配置的服务名称，未配置时使用应用名称
func (c Config) ServiceNameOrDefault(appName string) string {
	if name := strings.TrimSpace(c.ServiceName); name != "" {
		return name
	}
	return strings.TrimSpace(appName)
}

// ServiceHostPort 解析 consul.service_address（host:port）
func (c Config) ServiceHostPort() (host string, port int, err error) {
	return parseConsulHostPort(c.ServiceAddress)
}

// CheckURL 返回 consul.check_address 配置的完整健康检查 URL
func (c Config) CheckURL() (string, error) {
	return parseConsulCheckURL(c.CheckAddress)
}

// DiscoveryURLSchemeClean 返回服务发现 URL 协议前缀，未配置时返回空字符串
func (c Config) DiscoveryURLSchemeClean() string {
	return strings.TrimSpace(c.DiscoveryURLScheme)
}

// validateConsulHostPort 校验 Consul host:port 地址格式
func validateConsulHostPort(value, field string) error {
	if strings.TrimSpace(value) == "" {
		return errors.Newf("%s 不能为空", field)
	}
	if _, _, err := parseConsulHostPort(value); err != nil {
		return errors.Wrap(err, "%s", field)
	}
	return nil
}

// validateConsulCheckURL 校验 Consul 健康检查 URL
func validateConsulCheckURL(value, field string) error {
	if strings.TrimSpace(value) == "" {
		return errors.Newf("%s 不能为空", field)
	}
	if _, err := parseConsulCheckURL(value); err != nil {
		return errors.Wrap(err, "%s", field)
	}
	return nil
}

// validateDiscoveryURLScheme 校验服务发现 URL 协议前缀
func validateDiscoveryURLScheme(value, field string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if value != "http://" && value != "https://" {
		return errors.Newf("%s 须为 http:// 或 https://", field)
	}
	return nil
}

// parseConsulHostPort 解析 host:port 为 host 与 port
func parseConsulHostPort(value string) (host string, port int, err error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", 0, errors.New("地址须为 host:port 格式")
	}
	host, portStr, splitErr := net.SplitHostPort(value)
	if splitErr != nil {
		return "", 0, errors.New("地址须为 host:port 格式")
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return "", 0, errors.New("地址 host 不能为空")
	}
	port, err = strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, errors.New("地址 port 无效")
	}
	return host, port, nil
}

// parseConsulCheckURL 解析并校验完整健康检查 URL
func parseConsulCheckURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	u, err := url.Parse(value)
	if err != nil {
		return "", errors.New("须为完整 URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("URL scheme 须为 http 或 https")
	}
	if strings.TrimSpace(u.Host) == "" {
		return "", errors.New("URL host 不能为空")
	}
	return value, nil
}

// ConfigSection 返回 consul 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &Config{}, func(c *Config, _ *config.Document) error {
		return c.Validate()
	})
}
