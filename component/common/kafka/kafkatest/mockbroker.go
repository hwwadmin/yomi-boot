package kafkatest

import (
	"context"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/kafka"
	"gitee.com/hq_project/yomi-boot/component/core/errors"

	kafkago "github.com/segmentio/kafka-go"
)

const defaultPartitions = 1

// MockBroker 表示可在测试中使用的内存 Kafka broker（底层为 kfake，对外仍通过 kafka-go 客户端访问）。
type MockBroker struct {
	cluster *kfakeCluster
	t       testing.TB
}

// Option 配置 MockBroker 启动参数。
type Option func(*mockBrokerOptions)

// mockBrokerOptions 聚合 MockBroker 启动配置参数
type mockBrokerOptions struct {
	numBrokers int
	seedTopics []seedTopic
}

// seedTopic 表示一组预创建主题及其分区数
type seedTopic struct {
	partitions int
	topics     []string
}

// NumBrokers 指定 broker 节点数量，默认 1。
func NumBrokers(n int) Option {
	return func(o *mockBrokerOptions) {
		if n > 0 {
			o.numBrokers = n
		}
	}
}

// SeedTopics 预创建主题，partitions 为每个主题的分区数。
func SeedTopics(partitions int, topics ...string) Option {
	return func(o *mockBrokerOptions) {
		if len(topics) == 0 {
			return
		}
		if partitions <= 0 {
			partitions = defaultPartitions
		}
		o.seedTopics = append(o.seedTopics, seedTopic{
			partitions: partitions,
			topics:     append([]string(nil), topics...),
		})
	}
}

// NewMockBroker 启动内存 broker，测试结束时自动关闭。
func NewMockBroker(t testing.TB, opts ...Option) *MockBroker {
	t.Helper()

	options := mockBrokerOptions{numBrokers: 1}
	for _, opt := range opts {
		opt(&options)
	}

	clusterOpts := buildKfakeOpts(options)
	cluster := newKfakeCluster(t, clusterOpts)

	return &MockBroker{
		cluster: cluster,
		t:       t,
	}
}

// buildKfakeOpts 将 MockBroker 配置转换为 kfake 集群选项
func buildKfakeOpts(options mockBrokerOptions) []kfakeOpt {
	clusterOpts := make([]kfakeOpt, 0, 1+len(options.seedTopics))
	clusterOpts = append(clusterOpts, kfakeOptNumBrokers(options.numBrokers))
	for _, seed := range options.seedTopics {
		clusterOpts = append(clusterOpts, kfakeOptSeedTopics(int32(seed.partitions), seed.topics...))
	}
	return clusterOpts
}

// Close 关闭 broker 并释放资源。
func (b *MockBroker) Close() {
	if b == nil || b.cluster == nil {
		return
	}
	b.cluster.close()
}

// Brokers 返回 broker 地址列表。
func (b *MockBroker) Brokers() []string {
	if b == nil || b.cluster == nil {
		return nil
	}
	return b.cluster.addrs()
}

// Addr 返回首个 broker 地址。
func (b *MockBroker) Addr() string {
	brokers := b.Brokers()
	if len(brokers) == 0 {
		return ""
	}
	return brokers[0]
}

// Publish 向指定主题分区 0 写入消息（通过 kafka-go Conn）。
func (b *MockBroker) Publish(ctx context.Context, topic string, msgs ...kafkago.Message) error {
	if b == nil || b.cluster == nil {
		return errors.New("MockBroker 未初始化")
	}
	if topic == "" {
		return errors.New("topic 不能为空")
	}
	if len(msgs) == 0 {
		return nil
	}

	conn, err := kafkago.DialLeader(ctx, "tcp", b.Addr(), topic, 0)
	if err != nil {
		return errors.Wrap(err, "连接 topic %s 失败", topic)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.WriteMessages(msgs...); err != nil {
		return errors.Wrap(err, "写入 topic %s 失败", topic)
	}
	return nil
}

// ConnectionConfig 返回可用于 gitee.com/hq_project/yomi-boot kafka 组件的最小连接配置。
func (b *MockBroker) ConnectionConfig() kafka.ConnectionConfig {
	return kafka.ConnectionConfig{
		Brokers:            b.Brokers(),
		ClientID:           "yomi-kafka-test",
		DialTimeoutSeconds: 5,
		SecurityProtocol:   "PLAINTEXT",
	}
}

// PublisherConfig 返回可用于 kafka.NewPublisher 的最小生产者配置。
func (b *MockBroker) PublisherConfig() kafka.PublisherConfig {
	requiredAcks := -1
	return kafka.PublisherConfig{
		Connection:          b.ConnectionConfig(),
		RequiredAcks:        &requiredAcks,
		BatchSize:           1,
		BatchTimeoutSeconds: 1,
	}
}

// ConsumerManagerConfig 返回可用于 kafka.NewConsumerManager 的最小消费者配置。
func (b *MockBroker) ConsumerManagerConfig() kafka.ConsumerManagerConfig {
	return kafka.ConsumerManagerConfig{
		Connection:     b.ConnectionConfig(),
		MinBytes:       1,
		MaxBytes:       10e6,
		MaxWaitSeconds: 1,
		StartOffset:    "earliest",
	}
}

// ConsumerConfig 返回指定主题与消费组的消费参数。
func (b *MockBroker) ConsumerConfig(topics []string, groupID string) kafka.ConsumerConfig {
	return kafka.ConsumerConfig{
		Topics:   append([]string(nil), topics...),
		GroupID:  groupID,
		MinBytes: 1,
		MaxBytes: 10e6,
		MaxWait:  100 * time.Millisecond,
	}
}

// NewWriter 创建指向 mock broker 的 kafka-go Writer。
func (b *MockBroker) NewWriter(topic string) *kafkago.Writer {
	return &kafkago.Writer{
		Addr:         kafkago.TCP(b.Addr()),
		Topic:        topic,
		BatchSize:    1,
		BatchTimeout: 10 * time.Millisecond,
		RequiredAcks: kafkago.RequireAll,
	}
}

// NewReader 创建指向 mock broker 的 kafka-go Reader。
func (b *MockBroker) NewReader(cfg kafkago.ReaderConfig) *kafkago.Reader {
	if len(cfg.Brokers) == 0 {
		cfg.Brokers = b.Brokers()
	}
	if cfg.MinBytes == 0 {
		cfg.MinBytes = 1
	}
	if cfg.MaxBytes == 0 {
		cfg.MaxBytes = 10e6
	}
	if cfg.MaxWait == 0 {
		cfg.MaxWait = 100 * time.Millisecond
	}
	return kafkago.NewReader(cfg)
}

// NewPublisher 创建 kafka 生产者并在测试结束时释放。
func NewPublisher(t testing.TB, broker *MockBroker) kafka.PublisherService {
	t.Helper()
	if broker == nil {
		t.Fatal("broker 不能为空")
		return nil
	}

	publisher, err := kafka.NewPublisher(broker.PublisherConfig())
	if err != nil {
		t.Fatalf("初始化 kafka 生产者失败: %v", err)
	}
	t.Cleanup(func() {
		_ = publisher.Close()
	})
	return publisher
}

// NewConsumerManager 创建 kafka 消费者管理器并在测试结束时释放。
func NewConsumerManager(t testing.TB, broker *MockBroker) kafka.ConsumerService {
	t.Helper()
	if broker == nil {
		t.Fatal("broker 不能为空")
		return nil
	}

	consumer, err := kafka.NewConsumerManager(broker.ConsumerManagerConfig())
	if err != nil {
		t.Fatalf("初始化 kafka 消费者管理器失败: %v", err)
	}
	t.Cleanup(func() {
		_ = consumer.Close()
	})
	return consumer
}
