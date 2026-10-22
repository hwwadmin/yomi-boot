package kafka

import (
	"testing"

	kafkago "github.com/segmentio/kafka-go"
)

// TestConnectionConfigValidate 验证连接配置的 broker、超时与安全字段校验
func TestConnectionConfigValidate(t *testing.T) {
	base := ConnectionConfig{
		Brokers:            []string{"127.0.0.1:9092"},
		DialTimeoutSeconds: 1,
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("合法连接配置应通过校验，实际 %v", err)
	}

	cases := []struct {
		name string
		cfg  ConnectionConfig
	}{
		{name: "empty brokers", cfg: ConnectionConfig{DialTimeoutSeconds: 1}},
		{name: "blank broker", cfg: ConnectionConfig{Brokers: []string{" "}, DialTimeoutSeconds: 1}},
		{name: "zero dial timeout", cfg: ConnectionConfig{Brokers: []string{"127.0.0.1:9092"}}},
		{
			name: "unsupported security protocol",
			cfg: ConnectionConfig{
				Brokers:            []string{"127.0.0.1:9092"},
				DialTimeoutSeconds: 1,
				SecurityProtocol:   "UNKNOWN",
			},
		},
		{
			name: "missing sasl credentials",
			cfg: ConnectionConfig{
				Brokers:            []string{"127.0.0.1:9092"},
				DialTimeoutSeconds: 1,
				SecurityProtocol:   "SASL_PLAINTEXT",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cfg.Validate(); err == nil {
				t.Fatal("应校验失败")
			}
		})
	}
}

// TestPublisherConfigRequiredAcksOrDefault 验证 required_acks 未配置时默认 acks=all
func TestPublisherConfigRequiredAcksOrDefault(t *testing.T) {
	if got := (PublisherConfig{}).RequiredAcksOrDefault(); got != -1 {
		t.Fatalf("未配置 required_acks 期望默认 -1，实际 %d", got)
	}
	cases := []struct {
		name  string
		value int
	}{
		{name: "all", value: -1},
		{name: "none", value: 0},
		{name: "leader", value: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value := tc.value
			if got := (PublisherConfig{RequiredAcks: &value}).RequiredAcksOrDefault(); got != tc.value {
				t.Fatalf("显式配置期望 %d，实际 %d", tc.value, got)
			}
		})
	}
}

// TestPublisherConfigValidateRequiredAcks 验证 required_acks 的合法域
func TestPublisherConfigValidateRequiredAcks(t *testing.T) {
	base := PublisherConfig{
		Connection: ConnectionConfig{
			Brokers:            []string{"127.0.0.1:9092"},
			DialTimeoutSeconds: 1,
		},
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("未配置 required_acks 应使用默认值并通过校验，实际 %v", err)
	}

	for _, value := range []int{-1, 0, 1} {
		requiredAcks := value
		config := base
		config.RequiredAcks = &requiredAcks
		if err := config.Validate(); err != nil {
			t.Fatalf("required_acks=%d 应通过校验，实际 %v", value, err)
		}
	}
	for _, value := range []int{-2, 2} {
		requiredAcks := value
		config := base
		config.RequiredAcks = &requiredAcks
		if err := config.Validate(); err == nil {
			t.Fatalf("required_acks=%d 应校验失败", value)
		}
	}
}

// TestPublisherConfigValidate 验证生产者配置只校验生产者字段
func TestPublisherConfigValidate(t *testing.T) {
	base := PublisherConfig{
		Connection: ConnectionConfig{
			Brokers:            []string{"127.0.0.1:9092"},
			DialTimeoutSeconds: 1,
		},
	}

	invalidBatchSize := base
	invalidBatchSize.BatchSize = -1
	if err := invalidBatchSize.Validate(); err == nil {
		t.Fatal("batch_size=-1 应校验失败")
	}

	invalidWorker := base
	invalidWorker.PublishWorker.WorkNum = -1
	if err := invalidWorker.Validate(); err == nil {
		t.Fatal("publish_worker.work_num=-1 应校验失败")
	}
}

// TestConsumerManagerConfigValidate 验证消费者管理器配置只校验消费者字段
func TestConsumerManagerConfigValidate(t *testing.T) {
	base := ConsumerManagerConfig{
		Connection: ConnectionConfig{
			Brokers:            []string{"127.0.0.1:9092"},
			DialTimeoutSeconds: 1,
		},
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("合法消费者配置应通过校验，实际 %v", err)
	}

	invalidMinBytes := base
	invalidMinBytes.MinBytes = -1
	if err := invalidMinBytes.Validate(); err == nil {
		t.Fatal("min_bytes=-1 应校验失败")
	}

	invalidStartOffset := base
	invalidStartOffset.StartOffset = "middle"
	if err := invalidStartOffset.Validate(); err == nil {
		t.Fatal("start_offset=middle 应校验失败")
	}
}

// TestRequiredAcksFromConfig 验证配置值到 kafka-go 确认级别的映射
func TestRequiredAcksFromConfig(t *testing.T) {
	cases := []struct {
		value int
		want  kafkago.RequiredAcks
	}{
		{value: -1, want: kafkago.RequireAll},
		{value: 0, want: kafkago.RequireNone},
		{value: 1, want: kafkago.RequireOne},
	}
	for _, tc := range cases {
		if got := requiredAcksFromConfig(tc.value); got != tc.want {
			t.Fatalf("required_acks=%d 期望 %v，实际 %v", tc.value, tc.want, got)
		}
	}
}
