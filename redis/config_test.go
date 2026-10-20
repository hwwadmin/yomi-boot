// Package redis 配置单测
//
// @author zwei
package redis

import (
	"testing"
	"time"
)

// TestConfigCommandTimeout_Default 验证未配置或非正数时指令超时取默认 3s
func TestConfigCommandTimeout_Default(t *testing.T) {
	t.Parallel()

	if got := (Config{}).CommandTimeout(); got != 3*time.Second {
		t.Fatalf("未配置 CommandTimeout=%v want 3s", got)
	}
	if got := (Config{CommandTimeoutMs: 0}).CommandTimeout(); got != 3*time.Second {
		t.Fatalf("CommandTimeoutMs=0 应取默认值，实际 %v", got)
	}
}

// TestConfigCommandTimeout_Override 验证显式配置优先生效
func TestConfigCommandTimeout_Override(t *testing.T) {
	t.Parallel()

	cfg := Config{CommandTimeoutMs: 1500}
	if got := cfg.CommandTimeout(); got != 1500*time.Millisecond {
		t.Fatalf("CommandTimeout=%v want 1500ms", got)
	}
}

// TestConfigValidate_RejectsNegativeCommandTimeout 验证负数指令超时被拒绝
func TestConfigValidate_RejectsNegativeCommandTimeout(t *testing.T) {
	t.Parallel()

	enabled := true
	cfg := Config{Enabled: &enabled, Addr: "127.0.0.1:6379", CommandTimeoutMs: -1}
	if err := cfg.Validate(); err == nil {
		t.Fatal("期望 command_timeout_ms 为负数时校验失败")
	}
}
