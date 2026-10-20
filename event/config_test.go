package event

import (
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/event/cfg"
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

// TestConfigCleanupDefaults 校验 outbox/inbox 清理默认值与独立配置
func TestConfigCleanupDefaults(t *testing.T) {
	runtime := runtimeFrom(Config{}, "app")
	assertCleanupRuntime(t, "outbox", runtime.OutboxCleanup, false, 30*24*time.Hour, time.Hour, 500, 20)
	assertCleanupRuntime(t, "inbox", runtime.InboxCleanup, false, 30*24*time.Hour, time.Hour, 500, 20)

	outboxEnabled := true
	config := Config{
		OutboxCleanupEnabled:     &outboxEnabled,
		OutboxRetentionDays:      10,
		OutboxCleanupFrequencyMS: 60000,
		OutboxCleanupBatchSize:   100,
		OutboxCleanupMaxBatches:  5,
	}
	runtime = runtimeFrom(config, "app")
	assertCleanupRuntime(t, "outbox", runtime.OutboxCleanup, true, 10*24*time.Hour, time.Minute, 100, 5)
	assertCleanupRuntime(t, "inbox", runtime.InboxCleanup, false, 30*24*time.Hour, time.Hour, 500, 20)
}

// assertCleanupRuntime 校验单表清理运行时配置
func assertCleanupRuntime(
	t *testing.T,
	name string,
	cleanup cfg.CleanupRuntime,
	enabled bool,
	retention time.Duration,
	frequency time.Duration,
	batchSize int,
	maxBatches int,
) {
	t.Helper()
	if cleanup.Enabled != enabled {
		t.Fatalf("%s enabled 期望 %v，实际 %v", name, enabled, cleanup.Enabled)
	}
	if cleanup.Retention != retention {
		t.Fatalf("%s retention 期望 %s，实际 %s", name, retention, cleanup.Retention)
	}
	if cleanup.Frequency != frequency {
		t.Fatalf("%s frequency 期望 %s，实际 %s", name, frequency, cleanup.Frequency)
	}
	if cleanup.BatchSize != batchSize {
		t.Fatalf("%s batchSize 期望 %d，实际 %d", name, batchSize, cleanup.BatchSize)
	}
	if cleanup.MaxBatches != maxBatches {
		t.Fatalf("%s maxBatches 期望 %d，实际 %d", name, maxBatches, cleanup.MaxBatches)
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
