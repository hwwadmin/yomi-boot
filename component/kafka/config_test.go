// Package kafka 的生产者确认级别配置测试
//
// @author zwei
package kafka

import (
	"testing"

	kafkago "github.com/segmentio/kafka-go"
)

// TestConfigRequiredAcksOrDefault 验证 required_acks 未配置时默认 acks=all
func TestConfigRequiredAcksOrDefault(t *testing.T) {
	if got := (Config{}).RequiredAcksOrDefault(); got != -1 {
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
			if got := (Config{RequiredAcks: &value}).RequiredAcksOrDefault(); got != tc.value {
				t.Fatalf("显式配置期望 %d，实际 %d", tc.value, got)
			}
		})
	}
}

// TestConfigValidateRequiredAcks 验证 required_acks 的合法域
func TestConfigValidateRequiredAcks(t *testing.T) {
	enabled := true
	base := Config{
		Enabled:            &enabled,
		Brokers:            []string{"127.0.0.1:9092"},
		DialTimeoutSeconds: 1,
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
