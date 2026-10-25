// Package consul 的 HTTP 超时配置单测
//
// @author zwei
package consul

import (
	"strings"
	"testing"
	"time"
)

// TestHTTPTimeoutOrDefault 验证未配置时默认 10s，显式配置时按秒生效
func TestHTTPTimeoutOrDefault(t *testing.T) {
	t.Parallel()

	if got := (Config{}).HTTPTimeoutOrDefault(); got != 10*time.Second {
		t.Fatalf("默认超时期望 10s，实际 %s", got)
	}
	if got := (Config{HTTPTimeoutSeconds: 3}).HTTPTimeoutOrDefault(); got != 3*time.Second {
		t.Fatalf("显式超时期望 3s，实际 %s", got)
	}
}

// TestDeregisterCriticalServiceAfter 验证未配置时默认 30 分钟，显式配置时按秒生效
func TestDeregisterCriticalServiceAfter(t *testing.T) {
	t.Parallel()

	if got := (Config{}).DeregisterCriticalServiceAfter(); got != 30*time.Minute {
		t.Fatalf("默认注销时长期望 30m，实际 %s", got)
	}
	if got := (Config{DeregisterAfterSeconds: 600}).DeregisterCriticalServiceAfter(); got != 10*time.Minute {
		t.Fatalf("显式注销时长期望 10m，实际 %s", got)
	}
}

// TestDeregisterEnabledOrDefault 验证未配置时默认启用，显式关闭时返回 false
func TestDeregisterEnabledOrDefault(t *testing.T) {
	t.Parallel()

	if !(Config{}).DeregisterEnabledOrDefault() {
		t.Fatal("未配置时应默认启用自动注销")
	}
	disabled := false
	if (Config{DeregisterEnabled: &disabled}).DeregisterEnabledOrDefault() {
		t.Fatal("显式关闭时应返回 false")
	}
}

// TestConfigValidateDeregisterCriticalServiceAfter 验证注销时长负数或小于 1 分钟时校验失败
func TestConfigValidateDeregisterCriticalServiceAfter(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Addr:           "127.0.0.1:8500",
		ServiceID:      "svc-1",
		ServiceAddress: "127.0.0.1:8080",
		CheckAddress:   "http://127.0.0.1:8080/health",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("合法配置不应失败: %v", err)
	}

	cfg.DeregisterAfterSeconds = -1
	if err := cfg.Validate(); err == nil {
		t.Fatal("deregister_after_seconds 为负数时应失败")
	}

	cfg.DeregisterAfterSeconds = 30
	if err := cfg.Validate(); err == nil {
		t.Fatal("deregister_after_seconds 小于 60 时应失败")
	}

	cfg.DeregisterAfterSeconds = 60
	if err := cfg.Validate(); err != nil {
		t.Fatalf("60 秒应通过: %v", err)
	}

	disabled := false
	cfg.DeregisterEnabled = &disabled
	cfg.DeregisterAfterSeconds = 30
	if err := cfg.Validate(); err != nil {
		t.Fatalf("关闭自动注销时秒数应被忽略: %v", err)
	}
}

// TestConfigValidateHTTPTimeout 验证 http_timeout_seconds 合法时通过、为负数时校验失败
func TestConfigValidateHTTPTimeout(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Addr:           "127.0.0.1:8500",
		ServiceID:      "svc-1",
		ServiceAddress: "127.0.0.1:8080",
		CheckAddress:   "http://127.0.0.1:8080/health",
	}
	// 其余字段合法时，未配置超时也应通过
	if err := cfg.Validate(); err != nil {
		t.Fatalf("合法配置不应失败: %v", err)
	}

	cfg.HTTPTimeoutSeconds = -1
	if err := cfg.Validate(); err == nil {
		t.Fatal("http_timeout_seconds 为负数时应失败")
	}
}

// TestConfigValidateRequiredFields 验证必填字段与缓存秒数的校验
func TestConfigValidateRequiredFields(t *testing.T) {
	t.Parallel()

	base := Config{
		ServiceID:      "svc-1",
		ServiceAddress: "127.0.0.1:8080",
		CheckAddress:   "http://127.0.0.1:8080/health",
	}
	tests := []struct {
		name   string
		mutate func(cfg *Config)
	}{
		{name: "addr 为空", mutate: func(cfg *Config) { cfg.Addr = "  " }},
		{name: "service_id 为空", mutate: func(cfg *Config) { cfg.ServiceID = "" }},
		{name: "discovery_cache_seconds 为负数", mutate: func(cfg *Config) { cfg.DiscoveryCacheSeconds = -1 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := base
			cfg.Addr = "127.0.0.1:8500"
			tt.mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatalf("%s 期望报错", tt.name)
			}
		})
	}
}

// TestConfigValidateEndpointFields 验证 service_address / check_address / discovery_url_scheme 的校验
func TestConfigValidateEndpointFields(t *testing.T) {
	t.Parallel()

	base := Config{
		Addr:           "127.0.0.1:8500",
		ServiceID:      "svc-1",
		ServiceAddress: "127.0.0.1:8080",
		CheckAddress:   "http://127.0.0.1:8080/health",
	}
	tests := []struct {
		name    string
		mutate  func(cfg *Config)
		wantErr bool
	}{
		{name: "service_address 缺端口", mutate: func(cfg *Config) { cfg.ServiceAddress = "127.0.0.1" }, wantErr: true},
		{name: "service_address 为空", mutate: func(cfg *Config) { cfg.ServiceAddress = " " }, wantErr: true},
		{name: "check_address 非法协议", mutate: func(cfg *Config) { cfg.CheckAddress = "ftp://127.0.0.1/health" }, wantErr: true},
		{name: "check_address 缺 host", mutate: func(cfg *Config) { cfg.CheckAddress = "http:///health" }, wantErr: true},
		{name: "check_address 为空", mutate: func(cfg *Config) { cfg.CheckAddress = "" }, wantErr: true},
		{name: "discovery_url_scheme 非法", mutate: func(cfg *Config) { cfg.DiscoveryURLScheme = "ftp://" }, wantErr: true},
		{name: "discovery_url_scheme https 合法", mutate: func(cfg *Config) { cfg.DiscoveryURLScheme = "https://" }, wantErr: false},
		{name: "discovery_url_scheme 留空合法", mutate: func(cfg *Config) { cfg.DiscoveryURLScheme = "" }, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := base
			tt.mutate(&cfg)
			err := cfg.Validate()
			if tt.wantErr && err == nil {
				t.Fatalf("%s 期望报错", tt.name)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("%s 期望合法，实际: %v", tt.name, err)
			}
		})
	}
}

// TestServiceNameOrDefault 验证服务名的配置优先与空白回退
func TestServiceNameOrDefault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		serviceName string
		appName     string
		want        string
	}{
		{name: "配置优先", serviceName: "order-svc", appName: "app", want: "order-svc"},
		{name: "未配置回退应用名", serviceName: "", appName: "app", want: "app"},
		{name: "空白回退应用名", serviceName: "   ", appName: "app", want: "app"},
		{name: "去除首尾空白", serviceName: " order-svc ", appName: "app", want: "order-svc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := Config{ServiceName: tt.serviceName}
			if got := cfg.ServiceNameOrDefault(tt.appName); got != tt.want {
				t.Fatalf("ServiceNameOrDefault 期望 %q，实际 %q", tt.want, got)
			}
		})
	}
}

// TestParseConsulHostPort 验证 host:port 解析的各失败分支
func TestParseConsulHostPort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		wantErr string
		wantH   string
		wantP   int
	}{
		{name: "空值", in: "  ", wantErr: "地址须为 host:port 格式"},
		{name: "缺端口", in: "127.0.0.1", wantErr: "地址须为 host:port 格式"},
		{name: "缺 host", in: ":8080", wantErr: "地址 host 不能为空"},
		{name: "端口非数字", in: "127.0.0.1:abc", wantErr: "地址 port 无效"},
		{name: "端口为 0", in: "127.0.0.1:0", wantErr: "地址 port 无效"},
		{name: "端口超范围", in: "127.0.0.1:70000", wantErr: "地址 port 无效"},
		{name: "合法", in: "127.0.0.1:8080", wantH: "127.0.0.1", wantP: 8080},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			host, port, err := parseConsulHostPort(tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("期望错误包含 %q，实际: %v", tt.wantErr, err)
				}
				return
			}
			if err != nil || host != tt.wantH || port != tt.wantP {
				t.Fatalf("期望 host=%s port=%d，实际 host=%s port=%d err=%v", tt.wantH, tt.wantP, host, port, err)
			}
		})
	}
}

// TestParseConsulCheckURL 验证健康检查 URL 解析的各失败分支
func TestParseConsulCheckURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		wantErr string
	}{
		{name: "空值", in: "", wantErr: "URL scheme 须为 http 或 https"},
		{name: "非法协议", in: "ftp://127.0.0.1/health", wantErr: "URL scheme 须为 http 或 https"},
		{name: "缺 host", in: "http:///health", wantErr: "URL host 不能为空"},
		{name: "含控制字符", in: "http://127.0.0.1/\x7f", wantErr: "须为完整 URL"},
		{name: "合法", in: "http://127.0.0.1:8080/health"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseConsulCheckURL(tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("期望错误包含 %q，实际: %v", tt.wantErr, err)
				}
				return
			}
			if err != nil || got != tt.in {
				t.Fatalf("期望原样返回 %q，实际 %q err=%v", tt.in, got, err)
			}
		})
	}
}

// TestDiscoveryStaleIfErrorOrDefault 验证陈旧兜底窗口的三态默认值
func TestDiscoveryStaleIfErrorOrDefault(t *testing.T) {
	t.Parallel()

	if got := (Config{}).DiscoveryStaleIfErrorOrDefault(); got != 300*time.Second {
		t.Fatalf("未配置默认 300s，实际 %s", got)
	}
	zero := 0
	if got := (Config{DiscoveryStaleIfErrorSeconds: &zero}).DiscoveryStaleIfErrorOrDefault(); got != 0 {
		t.Fatalf("显式 0 应关闭兜底，实际 %s", got)
	}
	custom := 42
	if got := (Config{DiscoveryStaleIfErrorSeconds: &custom}).DiscoveryStaleIfErrorOrDefault(); got != 42*time.Second {
		t.Fatalf("显式 42 期望 42s，实际 %s", got)
	}
}

// TestConfigValidateStaleIfError 验证陈旧兜底窗口的负数校验
func TestConfigValidateStaleIfError(t *testing.T) {
	t.Parallel()

	negative := -1
	cfg := Config{
		Addr:                         "127.0.0.1:8500",
		ServiceID:                    "svc-1",
		ServiceAddress:               "10.0.0.1:8080",
		CheckAddress:                 "http://10.0.0.1:8080/health",
		DiscoveryStaleIfErrorSeconds: &negative,
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "discovery_stale_if_error_seconds") {
		t.Fatalf("负数应校验失败，实际 %v", err)
	}
}
