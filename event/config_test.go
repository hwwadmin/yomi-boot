package event

import "testing"

// TestConfigValidateFrequencyMinimum 校验抓取与死信轮询频率不得低于 roundrobin 的最小间隔
func TestConfigValidateFrequencyMinimum(t *testing.T) {
	enabled := true
	cases := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "disabled accepts small value",
			cfg:  Config{FetchFrequencyMS: 1, DlqFrequencyMS: 1},
		},
		{
			name: "fetch default value",
			cfg:  Config{PublisherEnabled: &enabled},
		},
		{
			name:    "fetch below minimum",
			cfg:     Config{PublisherEnabled: &enabled, FetchFrequencyMS: 1},
			wantErr: true,
		},
		{
			name:    "fetch at minimum",
			cfg:     Config{PublisherEnabled: &enabled, FetchFrequencyMS: 50},
			wantErr: false,
		},
		{
			name:    "dlq below minimum",
			cfg:     Config{PublisherEnabled: &enabled, DlqFrequencyMS: 49},
			wantErr: true,
		},
		{
			name:    "dlq at minimum",
			cfg:     Config{PublisherEnabled: &enabled, DlqFrequencyMS: 50},
			wantErr: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}
