// Package consul 的 HTTP 超时配置单测
//
// @author zwei
package consul

import (
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
