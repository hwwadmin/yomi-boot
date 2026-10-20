package event

import (
	"testing"
	"time"
)

// TestConfigConsumerDefaults 校验消费重试与收件箱死信重投的默认值
func TestConfigConsumerDefaults(t *testing.T) {
	config := Config{}
	if got := config.ConsumerMaxRetryOrDefault(); got != 20 {
		t.Fatalf("consumer_max_retry 默认值期望 20，实际 %d", got)
	}
	if got := config.InboxDlqFrequencyOrDefault(); got != 30*time.Second {
		t.Fatalf("inbox_dlq_frequency_ms 默认值期望 30s，实际 %s", got)
	}
	if got := config.InboxDlqSizeOrDefault(); got != 100 {
		t.Fatalf("inbox_dlq_size 默认值期望 100，实际 %d", got)
	}
}

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
