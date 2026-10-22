package kafkaplugin

import (
	"strings"
	"testing"
)

// boolPtr 返回 bool 指针，便于构造角色开关
func boolPtr(v bool) *bool {
	return &v
}

// intPtr 返回 int 指针，便于构造 required_acks
func intPtr(v int) *int {
	return &v
}

// validKafkaBootConfig 返回满足连接校验的最小配置
func validKafkaBootConfig() BootConfig {
	return BootConfig{
		Brokers:            []string{"127.0.0.1:9092"},
		DialTimeoutSeconds: 1,
	}
}

// TestBootConfigRoleDefaults 验证生产者与消费者开关未配置时默认关闭
func TestBootConfigRoleDefaults(t *testing.T) {
	cfg := BootConfig{}
	if cfg.PublisherEnabledOrDefault() {
		t.Fatal("publisher_enabled 未配置时应默认关闭")
	}
	if cfg.ConsumerEnabledOrDefault() {
		t.Fatal("consumer_enabled 未配置时应默认关闭")
	}
}

// TestBootConfigValidateDisabled 验证两端均未启用时整段跳过校验
func TestBootConfigValidateDisabled(t *testing.T) {
	cfg := BootConfig{
		StartOffset:   "middle",
		RequiredAcks:  intPtr(2),
		PublishWorker: PublishWorkerBootConfig{WorkNum: -1},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("两端均未启用时不应校验，实际 %v", err)
	}
}

// TestBootConfigValidatePublisherOnly 验证只启用生产者时不校验消费者字段
func TestBootConfigValidatePublisherOnly(t *testing.T) {
	cfg := validKafkaBootConfig()
	cfg.PublisherEnabled = boolPtr(true)
	cfg.StartOffset = "middle"
	cfg.MinBytes = -1
	cfg.MaxWaitSeconds = -1

	if err := cfg.Validate(); err != nil {
		t.Fatalf("只启用生产者时消费者字段不应影响校验，实际 %v", err)
	}
}

// TestBootConfigValidateConsumerOnly 验证只启用消费者时不校验生产者字段
func TestBootConfigValidateConsumerOnly(t *testing.T) {
	cfg := validKafkaBootConfig()
	cfg.ConsumerEnabled = boolPtr(true)
	cfg.RequiredAcks = intPtr(2)
	cfg.BatchSize = -1
	cfg.PublishWorker = PublishWorkerBootConfig{WorkNum: -1}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("只启用消费者时生产者字段不应影响校验，实际 %v", err)
	}
}

// TestBootConfigValidateBothEnabled 验证两端启用时分别报出对应角色的配置错误
func TestBootConfigValidateBothEnabled(t *testing.T) {
	t.Run("publisher invalid", func(t *testing.T) {
		cfg := validKafkaBootConfig()
		cfg.PublisherEnabled = boolPtr(true)
		cfg.ConsumerEnabled = boolPtr(true)
		cfg.RequiredAcks = intPtr(2)

		err := cfg.Validate()
		if err == nil {
			t.Fatal("required_acks=2 应校验失败")
		}
		if !strings.Contains(err.Error(), SectionKey+".publisher") {
			t.Fatalf("错误信息应包含 %s.publisher，实际 %v", SectionKey, err)
		}
	})

	t.Run("consumer invalid", func(t *testing.T) {
		cfg := validKafkaBootConfig()
		cfg.PublisherEnabled = boolPtr(true)
		cfg.ConsumerEnabled = boolPtr(true)
		cfg.StartOffset = "middle"

		err := cfg.Validate()
		if err == nil {
			t.Fatal("start_offset=middle 应校验失败")
		}
		if !strings.Contains(err.Error(), SectionKey+".consumer") {
			t.Fatalf("错误信息应包含 %s.consumer，实际 %v", SectionKey, err)
		}
	})
}
