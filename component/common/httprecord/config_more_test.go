// HTTP 访问记录配置默认值与校验的单元测试
//
// @author zwei
package httprecord

import (
	"testing"
)

func intPtr(v int) *int    { return &v }
func boolPtr(v bool) *bool { return &v }

// TestConfigProviderOrDefault 验证提供商默认值与规范化
func TestConfigProviderOrDefault(t *testing.T) {
	cases := []struct {
		provider string
		want     string
	}{
		{provider: "", want: ProviderMemory},
		{provider: "  ", want: ProviderMemory},
		{provider: "Kafka", want: "kafka"},
		{provider: " redis_stream ", want: "redis_stream"},
	}
	for _, tc := range cases {
		if got := (Config{Provider: tc.provider}).ProviderOrDefault(); got != tc.want {
			t.Fatalf("Provider=%q 期望 %q，实际 %q", tc.provider, tc.want, got)
		}
	}
}

// TestConfigTopicStreamDefaults 验证主题与 Stream 默认值
func TestConfigTopicStreamDefaults(t *testing.T) {
	if got := (Config{}).KafkaTopicOrDefault(); got != "yomi.http.record" {
		t.Fatalf("KafkaTopic 默认不符: %q", got)
	}
	if got := (Config{KafkaTopic: " custom "}).KafkaTopicOrDefault(); got != "custom" {
		t.Fatalf("KafkaTopic 裁剪不符: %q", got)
	}
	if got := (Config{}).StreamOrDefault(); got != "yomi.http.record" {
		t.Fatalf("Stream 默认不符: %q", got)
	}
	if got := (Config{Stream: "s1"}).StreamOrDefault(); got != "s1" {
		t.Fatalf("Stream 覆盖不符: %q", got)
	}
}

// TestConsumerAndMemoryDefaults 验证消费者与内存队列默认值
func TestConsumerAndMemoryDefaults(t *testing.T) {
	if (ConsumerConfig{}).EnabledOrDefault() {
		t.Fatal("未配置 Enabled 默认应为 false")
	}
	if !(ConsumerConfig{Enabled: boolPtr(true)}).EnabledOrDefault() {
		t.Fatal("Enabled=true 应生效")
	}
	if (ConsumerConfig{Enabled: boolPtr(false)}).EnabledOrDefault() {
		t.Fatal("Enabled=false 应生效")
	}
	if got := (MemoryConfig{}).QueueSizeOrDefault(); got != 1024 {
		t.Fatalf("QueueSize 默认期望 1024，实际 %d", got)
	}
	if got := (MemoryConfig{QueueSize: 8}).QueueSizeOrDefault(); got != 8 {
		t.Fatalf("QueueSize 覆盖期望 8，实际 %d", got)
	}
	if got := (MemoryConfig{}).WorkersOrDefault(); got != 2 {
		t.Fatalf("Workers 默认期望 2，实际 %d", got)
	}
	if got := (MemoryConfig{Workers: 5}).WorkersOrDefault(); got != 5 {
		t.Fatalf("Workers 覆盖期望 5，实际 %d", got)
	}
}

// TestResolveBodySizeLimit 验证体积上限解析：nil 与非正数用默认 1MB，正值透传
func TestResolveBodySizeLimit(t *testing.T) {
	if got := (Config{}).MaxRequestBodySizeOrDefault(); got != defaultHTTPRecordMaxRequestBodySize {
		t.Fatalf("未配置请求体上限期望 %d，实际 %d", defaultHTTPRecordMaxRequestBodySize, got)
	}
	if got := (Config{MaxRequestBodySize: intPtr(0)}).MaxRequestBodySizeOrDefault(); got != defaultHTTPRecordMaxRequestBodySize {
		t.Fatalf("显式 0 应回落到默认上限，实际 %d", got)
	}
	if got := (Config{MaxResponseBodySize: intPtr(16)}).MaxResponseBodySizeOrDefault(); got != 16 {
		t.Fatalf("显式 16 期望透传，实际 %d", got)
	}
}

// TestConfigValidateBranches 验证配置校验的各错误分支
func TestConfigValidateBranches(t *testing.T) {
	valid := Config{Provider: ProviderKafka}
	if err := valid.Validate(); err != nil {
		t.Fatalf("合法配置不应报错: %v", err)
	}

	cases := []struct {
		name string
		cfg  Config
	}{
		{name: "skip_paths 含首尾空格", cfg: Config{SkipPaths: []string{" /a"}}},
		{name: "请求体上限为负", cfg: Config{MaxRequestBodySize: intPtr(-1)}},
		{name: "请求体上限为 0", cfg: Config{MaxRequestBodySize: intPtr(0)}},
		{name: "响应体上限为负", cfg: Config{MaxResponseBodySize: intPtr(-1)}},
		{name: "响应体上限为 0", cfg: Config{MaxResponseBodySize: intPtr(0)}},
		{name: "provider 不支持", cfg: Config{Provider: "nsq"}},
		{name: "memory 关闭消费", cfg: Config{Provider: ProviderMemory, Consumer: ConsumerConfig{Enabled: boolPtr(false)}}},
		{name: "max_retry 为负", cfg: Config{Consumer: ConsumerConfig{MaxRetry: intPtr(-1)}}},
		{name: "queue_size 为负", cfg: Config{Memory: MemoryConfig{QueueSize: -1}}},
		{name: "workers 为负", cfg: Config{Memory: MemoryConfig{Workers: -1}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cfg.Validate(); err == nil {
				t.Fatalf("%s 应返回错误", tc.name)
			}
		})
	}
}
