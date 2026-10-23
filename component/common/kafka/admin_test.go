package kafka_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/kafka"
	"gitee.com/hq_project/yomi-boot/component/common/kafka/kafkatest"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// TestAdminMetadataAndListTopics 验证集群元数据与 topic 列表查询
func TestAdminMetadataAndListTopics(t *testing.T) {
	broker := kafkatest.NewMockBroker(t, kafkatest.SeedTopics(1, "orders", "events"))
	admin := kafkatest.NewAdmin(t, broker)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	meta, err := admin.Metadata(ctx)
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if len(meta.Brokers) == 0 {
		t.Fatal("Metadata 返回的 broker 列表为空")
	}
	if meta.Controller.ID != meta.Brokers[0].ID {
		t.Fatalf("Controller ID = %d, want %d", meta.Controller.ID, meta.Brokers[0].ID)
	}

	topics, err := admin.ListTopics(ctx)
	if err != nil {
		t.Fatalf("ListTopics: %v", err)
	}
	if !slices.IsSorted(topics) {
		t.Fatalf("ListTopics 结果未按名称升序: %v", topics)
	}
	for _, want := range []string{"orders", "events"} {
		if !slices.Contains(topics, want) {
			t.Fatalf("ListTopics 缺少 topic %q: %v", want, topics)
		}
	}
}

// TestAdminCreateDescribeDeleteTopic 验证 topic 创建、详情查询与删除
func TestAdminCreateDescribeDeleteTopic(t *testing.T) {
	broker := kafkatest.NewMockBroker(t, kafkatest.SeedTopics(1, "orders"))
	admin := kafkatest.NewAdmin(t, broker)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const topic = "created-topic"
	if err := admin.CreateTopic(ctx, kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     2,
		ReplicationFactor: 1,
		ConfigEntries: map[string]string{
			"cleanup.policy": "compact",
		},
	}); err != nil {
		t.Fatalf("CreateTopic: %v", err)
	}

	detail, err := admin.DescribeTopic(ctx, topic)
	if err != nil {
		t.Fatalf("DescribeTopic: %v", err)
	}
	if detail.Name != topic {
		t.Fatalf("DescribeTopic Name = %q, want %q", detail.Name, topic)
	}
	if len(detail.Partitions) != 2 {
		t.Fatalf("DescribeTopic Partitions = %d, want 2", len(detail.Partitions))
	}

	if err := admin.CreateTopic(ctx, kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     2,
		ReplicationFactor: 1,
	}); !errors.Is(err, kafka.ErrTopicAlreadyExists) {
		t.Fatalf("重复创建 CreateTopic error = %v, want ErrTopicAlreadyExists", err)
	}

	if err := admin.DeleteTopic(ctx, topic); err != nil {
		t.Fatalf("DeleteTopic: %v", err)
	}
	if _, err := admin.DescribeTopic(ctx, topic); !errors.Is(err, kafka.ErrTopicNotFound) {
		t.Fatalf("删除后 DescribeTopic error = %v, want ErrTopicNotFound", err)
	}
	if err := admin.DeleteTopic(ctx, topic); !errors.Is(err, kafka.ErrTopicNotFound) {
		t.Fatalf("重复删除 DeleteTopic error = %v, want ErrTopicNotFound", err)
	}
}

// TestAdminDescribeTopicNotFound 验证查询不存在的 topic 返回 ErrTopicNotFound
func TestAdminDescribeTopicNotFound(t *testing.T) {
	broker := kafkatest.NewMockBroker(t, kafkatest.SeedTopics(1, "orders"))
	admin := kafkatest.NewAdmin(t, broker)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := admin.DescribeTopic(ctx, "missing"); !errors.Is(err, kafka.ErrTopicNotFound) {
		t.Fatalf("DescribeTopic error = %v, want ErrTopicNotFound", err)
	}
}

// TestAdminCreateTopicInvalidConfig 验证创建 topic 参数校验
func TestAdminCreateTopicInvalidConfig(t *testing.T) {
	broker := kafkatest.NewMockBroker(t, kafkatest.SeedTopics(1, "orders"))
	admin := kafkatest.NewAdmin(t, broker)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cases := []struct {
		name string
		cfg  kafka.TopicConfig
	}{
		{name: "empty topic", cfg: kafka.TopicConfig{NumPartitions: 1, ReplicationFactor: 1}},
		{name: "zero partitions", cfg: kafka.TopicConfig{Topic: "invalid", NumPartitions: 0, ReplicationFactor: 1}},
		{name: "zero replication", cfg: kafka.TopicConfig{Topic: "invalid", NumPartitions: 1, ReplicationFactor: 0}},
		{
			name: "blank config key",
			cfg: kafka.TopicConfig{
				Topic:             "invalid",
				NumPartitions:     1,
				ReplicationFactor: 1,
				ConfigEntries:     map[string]string{" ": "value"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := admin.CreateTopic(ctx, tc.cfg); err == nil {
				t.Fatal("CreateTopic 应校验失败")
			}
		})
	}
}

// TestAdminCloseIdempotent 验证 Close 可重复调用
func TestAdminCloseIdempotent(t *testing.T) {
	broker := kafkatest.NewMockBroker(t, kafkatest.SeedTopics(1, "orders"))
	admin, err := kafka.NewAdmin(context.Background(), broker.ConnectionConfig())
	if err != nil {
		t.Fatalf("NewAdmin: %v", err)
	}

	if err := admin.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := admin.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
