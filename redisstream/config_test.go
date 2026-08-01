// Package redisstream 配置单测
//
// @author zwei
package redisstream

import "testing"

func TestConfigValidate_DisabledSkips(t *testing.T) {
	t.Parallel()
	cfg := Config{}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("disabled config should pass: %v", err)
	}
}

func TestConfigValidate_RejectsNegative(t *testing.T) {
	t.Parallel()
	neg := int64(-1)
	enabled := true
	cfg := Config{
		Enabled:       &enabled,
		DefaultMaxLen: &neg,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for negative default_maxlen")
	}
}

func TestConfigDefaults(t *testing.T) {
	t.Parallel()
	enabled := true
	cfg := Config{Enabled: &enabled}
	if got := cfg.DefaultMaxLenOrDefault(); got != defaultMaxLen {
		t.Fatalf("DefaultMaxLenOrDefault=%d want %d", got, defaultMaxLen)
	}
	zero := int64(0)
	cfg.DefaultMaxLen = &zero
	if got := cfg.DefaultMaxLenOrDefault(); got != 0 {
		t.Fatalf("explicit 0 DefaultMaxLen should stay 0, got %d", got)
	}
	if cfg.CountOrDefault() != defaultCount {
		t.Fatalf("CountOrDefault=%d want %d", cfg.CountOrDefault(), defaultCount)
	}
}
