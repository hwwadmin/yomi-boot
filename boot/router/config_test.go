package router

import (
	"testing"
	"time"
)

// TestConfigTimeoutDefaults 验证未配置时使用默认超时
func TestConfigTimeoutDefaults(t *testing.T) {
	cfg := Config{Port: 8080}
	if got := cfg.ShutdownTimeout(); got != 10*time.Second {
		t.Fatalf("ShutdownTimeout = %v, want 10s", got)
	}
	if got := cfg.ReadHeaderTimeout(); got != 5*time.Second {
		t.Fatalf("ReadHeaderTimeout = %v, want 5s", got)
	}
}

// TestConfigTimeoutExplicit 验证显式配置优先
func TestConfigTimeoutExplicit(t *testing.T) {
	cfg := Config{Port: 8080, ShutdownTimeoutSeconds: 3, ReadHeaderTimeoutSeconds: 2}
	if got := cfg.ShutdownTimeout(); got != 3*time.Second {
		t.Fatalf("ShutdownTimeout = %v, want 3s", got)
	}
	if got := cfg.ReadHeaderTimeout(); got != 2*time.Second {
		t.Fatalf("ReadHeaderTimeout = %v, want 2s", got)
	}
}

// TestConfigNegativeTimeouts 验证负数超时被拒绝
func TestConfigNegativeTimeouts(t *testing.T) {
	if err := (Config{Port: 8080, ShutdownTimeoutSeconds: -1}).Validate(); err == nil {
		t.Fatal("负的 shutdown_timeout_seconds 应校验失败")
	}
	if err := (Config{Port: 8080, ReadHeaderTimeoutSeconds: -1}).Validate(); err == nil {
		t.Fatal("负的 read_header_timeout_seconds 应校验失败")
	}
}
