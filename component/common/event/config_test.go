package event

import (
	"strings"
	"testing"
	"time"

	"github.com/hwwadmin/yomi-boot/component/common/event/cfg"
)

// TestConfigConsumerDefaults 校验消费重试与收件箱死信重投的默认值
func TestConfigConsumerDefaults(t *testing.T) {
	config := Config{}
	if got := config.InboxDlqMaxRetryOrDefault(); got != 20 {
		t.Fatalf("inbox_dlq_max_retry 默认值期望 20，实际 %d", got)
	}
	if got := config.InboxDlqFrequencyOrDefault(); got != 30*time.Second {
		t.Fatalf("inbox_dlq_frequency_ms 默认值期望 30s，实际 %s", got)
	}
	if got := config.InboxDlqSizeOrDefault(); got != 100 {
		t.Fatalf("inbox_dlq_size 默认值期望 100，实际 %d", got)
	}
}

// TestConfigOutboxDlqDefaults 校验发件箱死信配置默认值
func TestConfigOutboxDlqDefaults(t *testing.T) {
	config := Config{}
	if got := config.OutboxDlqFrequencyOrDefault(); got != time.Second {
		t.Fatalf("outbox_dlq_frequency_ms 默认值期望 1s，实际 %s", got)
	}
	if got := config.OutboxDlqSizeOrDefault(); got != 5000 {
		t.Fatalf("outbox_dlq_size 默认值期望 5000，实际 %d", got)
	}
	if got := config.OutboxDlqMaxRetryOrDefault(); got != 30 {
		t.Fatalf("outbox_dlq_max_retry 默认值期望 30，实际 %d", got)
	}
}

// TestConfigCleanupDefaults 校验 outbox/inbox 清理默认值与独立配置
func TestConfigCleanupDefaults(t *testing.T) {
	runtime := runtimeFrom(Config{}, "app")
	assertCleanupRuntime(t, "outbox", runtime.OutboxCleanup, false, 30*24*time.Hour, time.Hour, 500, 20)
	assertCleanupRuntime(t, "inbox", runtime.InboxCleanup, false, 30*24*time.Hour, time.Hour, 500, 20)

	config := Config{
		OutboxCleanupEnabled:     true,
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
	cases := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "disabled accepts small value",
			cfg:  Config{FetchFrequencyMS: 1, OutboxDlqFrequencyMS: 1},
		},
		{
			name: "fetch default value",
			cfg:  Config{OutboxEnabled: true},
		},
		{
			name:    "fetch below minimum",
			cfg:     Config{OutboxEnabled: true, FetchFrequencyMS: 1},
			wantErr: true,
		},
		{
			name:    "fetch at minimum",
			cfg:     Config{OutboxEnabled: true, FetchFrequencyMS: 50},
			wantErr: false,
		},
		{
			name:    "dlq below minimum",
			cfg:     Config{OutboxEnabled: true, OutboxDlqFrequencyMS: 49},
			wantErr: true,
		},
		{
			name:    "dlq at minimum",
			cfg:     Config{OutboxEnabled: true, OutboxDlqFrequencyMS: 50},
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

// TestConfigValidateServiceNameLength 校验服务名按字符计不超过 service_name 列宽
func TestConfigValidateServiceNameLength(t *testing.T) {
	exact := strings.Repeat("名", maxServiceNameLen)
	over := strings.Repeat("名", maxServiceNameLen+1)

	if err := (Config{OutboxEnabled: true, ServiceName: exact}).Validate(); err != nil {
		t.Fatalf("100 个字符的服务名应通过: %v", err)
	}
	if err := (Config{OutboxEnabled: true, ServiceName: "  " + exact + "  "}).Validate(); err != nil {
		t.Fatalf("去掉首尾空白后不超过列宽的服务名应通过: %v", err)
	}
	if err := (Config{OutboxEnabled: true, ServiceName: strings.Repeat(" ", maxServiceNameLen+1)}).Validate(); err != nil {
		t.Fatalf("空白服务名会回退到默认名，不应报错: %v", err)
	}
	if err := (Config{ServiceName: over}).Validate(); err != nil {
		t.Fatalf("未启用时不校验服务名长度: %v", err)
	}

	err := (Config{InboxEnabled: true, ServiceName: over}).Validate()
	if err == nil {
		t.Fatal("超过 100 个字符的服务名应返回错误")
	}
	if !strings.Contains(err.Error(), "service_name") || !strings.Contains(err.Error(), "100") {
		t.Fatalf("错误信息应指出 service_name 和上限，实际 %v", err)
	}
}

// TestValidateCleanupConfig 验证单组清理配置的合法域
func TestValidateCleanupConfig(t *testing.T) {
	const minFrequencyMS = 50
	valid := []int{30, 3600000, 500, 20}
	if err := validateCleanupConfig("outbox_cleanup", minFrequencyMS, valid[0], valid[1], valid[2], valid[3]); err != nil {
		t.Fatalf("合法配置不应报错: %v", err)
	}
	// frequency_ms 为 0 表示未配置，允许
	if err := validateCleanupConfig("outbox_cleanup", minFrequencyMS, 30, 0, 500, 20); err != nil {
		t.Fatalf("未配置频率不应报错: %v", err)
	}

	cases := []struct {
		name          string
		retentionDays int
		frequencyMS   int
		batchSize     int
		maxBatches    int
	}{
		{name: "negative retention", retentionDays: -1, frequencyMS: 3600000, batchSize: 500, maxBatches: 20},
		{name: "negative frequency", retentionDays: 30, frequencyMS: -1, batchSize: 500, maxBatches: 20},
		{name: "frequency below minimum", retentionDays: 30, frequencyMS: 10, batchSize: 500, maxBatches: 20},
		{name: "negative batch size", retentionDays: 30, frequencyMS: 3600000, batchSize: -1, maxBatches: 20},
		{name: "negative max batches", retentionDays: 30, frequencyMS: 3600000, batchSize: 500, maxBatches: -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCleanupConfig("outbox_cleanup", minFrequencyMS, tc.retentionDays, tc.frequencyMS, tc.batchSize, tc.maxBatches)
			if err == nil {
				t.Fatalf("%s 应返回错误", tc.name)
			}
		})
	}
}

func intPtr(v int) *int { return &v }

func TestConfigExplicitValuesAndRejects(t *testing.T) {
	if (Config{}).ServiceNameOrDefault("  ") != defaultEventServiceName {
		t.Fatal("空服务名应回落到默认值")
	}
	retry := 3
	inboxRetry := 4
	explicit := Config{
		FetchFrequencyMS:  1500,
		FetchSize:         10,
		SentThreadCount:   2,
		OutboxDlqSize:     8,
		OutboxDlqMaxRetry: &retry,
		InboxDlqMaxRetry:  &inboxRetry,
		InboxDlqSize:      9,
		OutboxEnabled:     true,
	}
	if explicit.FetchFrequencyOrDefault() != 1500*time.Millisecond ||
		explicit.FetchSizeOrDefault() != 10 ||
		explicit.SentThreadCountOrDefault() != 2 ||
		explicit.OutboxDlqSizeOrDefault() != 8 ||
		explicit.OutboxDlqMaxRetryOrDefault() != 3 ||
		explicit.InboxDlqMaxRetryOrDefault() != 4 ||
		explicit.InboxDlqSizeOrDefault() != 9 {
		t.Fatalf("显式配置未生效: %+v", explicit)
	}

	rejects := []Config{
		{OutboxEnabled: true, FetchFrequencyMS: -1},
		{OutboxEnabled: true, FetchSize: -1},
		{OutboxEnabled: true, SentThreadCount: -1},
		{OutboxEnabled: true, OutboxDlqFrequencyMS: -1},
		{OutboxEnabled: true, OutboxDlqSize: -1},
		{OutboxEnabled: true, OutboxDlqMaxRetry: &retry, FetchFrequencyMS: 0},
		{OutboxEnabled: true, FetchTimeoutMinutes: -1},
		{OutboxEnabled: true, InboxDlqFrequencyMS: -1},
		{OutboxEnabled: true, InboxDlqSize: -1},
		{OutboxEnabled: true, OutboxRetentionDays: -1},
		{OutboxEnabled: true, InboxRetentionDays: -1},
		{OutboxEnabled: true, InboxDlqFrequencyMS: 1},
	}
	rejects[5].OutboxDlqMaxRetry = intPtr(-1)
	for i, cfg := range rejects {
		if err := cfg.Validate(); err == nil {
			t.Fatalf("第 %d 组非法配置应失败: %+v", i, cfg)
		}
	}
}
