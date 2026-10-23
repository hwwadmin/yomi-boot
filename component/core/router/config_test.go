package router

import (
	"testing"
	"time"
)

// TestConfigTimeoutDefaults 验证未配置时使用默认超时
func TestConfigTimeoutDefaults(t *testing.T) {
	cfg := Config{Port: 8080}
	if got := cfg.ReadTimeout(); got != 10*time.Second {
		t.Fatalf("ReadTimeout = %v, want 10s", got)
	}
	if got := cfg.WriteTimeout(); got != 15*time.Second {
		t.Fatalf("WriteTimeout = %v, want 15s", got)
	}
	if got := cfg.ReadHeaderTimeout(); got != 5*time.Second {
		t.Fatalf("ReadHeaderTimeout = %v, want 5s", got)
	}
	if got := cfg.IdleTimeout(); got != 90*time.Second {
		t.Fatalf("IdleTimeout = %v, want 90s", got)
	}
	if got := cfg.ShutdownTimeout(); got != 10*time.Second {
		t.Fatalf("ShutdownTimeout = %v, want 10s", got)
	}
	if got := cfg.RequestTimeout(); got != 13*time.Second {
		t.Fatalf("RequestTimeout = %v, want 13s（默认写超时 15s - 2s）", got)
	}
}

// TestConfigTimeoutExplicit 验证显式配置优先
func TestConfigTimeoutExplicit(t *testing.T) {
	cfg := Config{
		Port:                     8080,
		ReadTimeoutSeconds:       8,
		WriteTimeoutSeconds:      20,
		ReadHeaderTimeoutSeconds: 2,
		IdleTimeoutSeconds:       30,
		ShutdownTimeoutSeconds:   3,
	}
	if got := cfg.ReadTimeout(); got != 8*time.Second {
		t.Fatalf("ReadTimeout = %v, want 8s", got)
	}
	if got := cfg.WriteTimeout(); got != 20*time.Second {
		t.Fatalf("WriteTimeout = %v, want 20s", got)
	}
	if got := cfg.ReadHeaderTimeout(); got != 2*time.Second {
		t.Fatalf("ReadHeaderTimeout = %v, want 2s", got)
	}
	if got := cfg.IdleTimeout(); got != 30*time.Second {
		t.Fatalf("IdleTimeout = %v, want 30s", got)
	}
	if got := cfg.ShutdownTimeout(); got != 3*time.Second {
		t.Fatalf("ShutdownTimeout = %v, want 3s", got)
	}
	if got := cfg.RequestTimeout(); got != 18*time.Second {
		t.Fatalf("RequestTimeout = %v, want 18s", got)
	}
}

// TestConfigNegativeTimeouts 验证负数超时被拒绝
func TestConfigNegativeTimeouts(t *testing.T) {
	if err := (Config{Port: 8080, ReadTimeoutSeconds: -1}).Validate(); err == nil {
		t.Fatal("负的 read_timeout_seconds 应校验失败")
	}
	if err := (Config{Port: 8080, WriteTimeoutSeconds: -1}).Validate(); err == nil {
		t.Fatal("负的 write_timeout_seconds 应校验失败")
	}
	if err := (Config{Port: 8080, IdleTimeoutSeconds: -1}).Validate(); err == nil {
		t.Fatal("负的 idle_timeout_seconds 应校验失败")
	}
	if err := (Config{Port: 8080, ShutdownTimeoutSeconds: -1}).Validate(); err == nil {
		t.Fatal("负的 shutdown_timeout_seconds 应校验失败")
	}
	if err := (Config{Port: 8080, ReadHeaderTimeoutSeconds: -1}).Validate(); err == nil {
		t.Fatal("负的 read_header_timeout_seconds 应校验失败")
	}
}
