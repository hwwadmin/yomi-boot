package db

import (
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/shared/enums"
)

func TestSlowThreshold(t *testing.T) {
	t.Parallel()

	ms := func(v int) *int { return &v }

	cases := []struct {
		name string
		cfg  Config
		want time.Duration
	}{
		{name: "未配置默认500ms", cfg: Config{}, want: 500 * time.Millisecond},
		{name: "显式200ms", cfg: Config{SlowThresholdMs: ms(200)}, want: 200 * time.Millisecond},
		{name: "0关闭慢SQL日志", cfg: Config{SlowThresholdMs: ms(0)}, want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.cfg.SlowThreshold(); got != tc.want {
				t.Fatalf("SlowThreshold()=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestValidateSlowThresholdMs(t *testing.T) {
	t.Parallel()

	enabled := true
	neg := -1
	ok := 500

	base := Config{
		Enabled:                &enabled,
		Type:                   enums.MySQL,
		Host:                   "127.0.0.1",
		Port:                   3306,
		User:                   "root",
		Database:               "demo",
		MaxOpenConns:           10,
		MaxIdleConns:           5,
		ConnMaxLifetimeMinutes: 60,
		ConnMaxIdleTimeMinutes: 30,
	}

	t.Run("负数非法", func(t *testing.T) {
		t.Parallel()
		cfg := base
		cfg.SlowThresholdMs = &neg
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected error for negative slow_threshold_ms")
		}
	})

	t.Run("合法阈值", func(t *testing.T) {
		t.Parallel()
		cfg := base
		cfg.SlowThresholdMs = &ok
		if err := cfg.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("未配置合法", func(t *testing.T) {
		t.Parallel()
		if err := base.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestValidateMinIdleConns(t *testing.T) {
	t.Parallel()

	enabled := true
	base := Config{
		Enabled:                &enabled,
		Type:                   enums.MySQL,
		Host:                   "127.0.0.1",
		Port:                   3306,
		User:                   "root",
		Database:               "demo",
		MaxOpenConns:           20,
		MaxIdleConns:           10,
		ConnMaxLifetimeMinutes: 60,
		ConnMaxIdleTimeMinutes: 30,
	}

	t.Run("默认0合法", func(t *testing.T) {
		t.Parallel()
		if err := base.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("等于max_idle合法", func(t *testing.T) {
		t.Parallel()
		cfg := base
		cfg.MinIdleConns = 10
		if err := cfg.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("大于max_idle非法", func(t *testing.T) {
		t.Parallel()
		cfg := base
		cfg.MinIdleConns = 11
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("大于max_open非法", func(t *testing.T) {
		t.Parallel()
		cfg := base
		cfg.MaxIdleConns = 20
		cfg.MinIdleConns = 21
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("负数非法", func(t *testing.T) {
		t.Parallel()
		cfg := base
		cfg.MinIdleConns = -1
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected error")
		}
	})
}
