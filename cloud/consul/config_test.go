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

// TestConfigValidateHTTPTimeout 验证 http_timeout_seconds 合法时通过、为负数时校验失败
func TestConfigValidateHTTPTimeout(t *testing.T) {
	t.Parallel()

	enabled := true
	cfg := Config{
		Enabled:        &enabled,
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
