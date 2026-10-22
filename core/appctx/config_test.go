package appctx

import "testing"

// TestConfig_Normalize_DefaultTimezone 验证未配置时区时归一化为 UTC
func TestConfig_Normalize_DefaultTimezone(t *testing.T) {
	cfg := Config{Name: "svc", Timezone: "  "}
	cfg.Normalize()
	if cfg.Timezone != DefaultTimezone {
		t.Fatalf("default timezone want %s, got %s", DefaultTimezone, cfg.Timezone)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	cfg = Config{Name: "svc", Timezone: "utc"}
	cfg.Normalize()
	if cfg.Timezone != DefaultTimezone {
		t.Fatalf("lowercase utc want %s, got %s", DefaultTimezone, cfg.Timezone)
	}
}

// TestConfig_Validate_Timezone 验证时区配置校验规则
func TestConfig_Validate_Timezone(t *testing.T) {
	cases := []struct {
		name     string
		timezone string
		wantErr  bool
	}{
		{name: "utc", timezone: "UTC"},
		{name: "iana", timezone: "Asia/Shanghai"},
		{name: "local", timezone: "Local", wantErr: true},
		{name: "invalid", timezone: "Mars/Base", wantErr: true},
		{name: "empty", timezone: "", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{Name: "svc", Timezone: tc.timezone}
			err := cfg.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate(%q) error = %v, wantErr %v", tc.timezone, err, tc.wantErr)
			}
		})
	}
}
