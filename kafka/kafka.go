// Package kafka 提供 Kafka 客户端连接能力
//
// @author zwei
package kafka

import (
	"context"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/appctx"
	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/logger"
	"gitee.com/hq_project/yomi-boot/worker"

	kafkago "github.com/segmentio/kafka-go"
)

// KafkaRuntimeConfig 表示 Kafka 运行时配置
type KafkaRuntimeConfig struct {
	Brokers     []string        // Brokers 是 Kafka broker 地址列表
	ClientID    string          // ClientID 是客户端标识
	MinBytes    int             // MinBytes 是消费者单次拉取最小字节数
	MaxBytes    int             // MaxBytes 是消费者单次拉取最大字节数
	MaxWait     time.Duration   // MaxWait 是消费者拉取等待时长
	StartOffset int64           // StartOffset 是消费起始偏移量
	Dialer      *kafkago.Dialer // Dialer 是带 SASL 等能力的连接拨号器
}

// Components 聚合 Kafka 发布者与消费者管理器
type Components struct {
	Publisher PublisherService // Publisher 是 Kafka 消息发布者
	Consumer  ConsumerService  // Consumer 是 Kafka 消费者管理器
}

// NewComponents 根据 Kafka 配置创建并验证连接，返回发布者与消费者管理器
func NewComponents(cfg Config) (*Components, error) {
	runtimeCfg, err := bootstrap(cfg)
	if err != nil {
		return nil, err
	}

	writer, err := buildWriter(cfg, runtimeCfg.Brokers)
	if err != nil {
		return nil, err
	}

	syncPublisher := NewPublisher(writer)
	publishWorkerCfg := buildPublishWorkerConfig(cfg.PublishWorker)
	publishWorker, err := worker.NewWorker(publishWorkerCfg, context.Background())
	if err != nil {
		_ = syncPublisher.Close()
		return nil, err
	}
	publishWorker.Start()

	logger.InfoWithoutCtx(
		"初始化kafka链接成功",
		"brokers", strings.Join(runtimeCfg.Brokers, ","),
		"security_protocol", NormalizeSecurityProtocol(cfg.SecurityProtocol),
		"sasl_mechanism", NormalizeSASLMechanism(cfg.SASLMechanism),
		"batch_size", effectiveBatchSize(cfg.BatchSize),
		"batch_timeout_seconds", effectiveBatchTimeoutSeconds(cfg.BatchTimeoutSeconds),
		"publish_worker_num", publishWorkerCfg.WorkNum,
		"publish_worker_size", publishWorkerCfg.Size,
	)

	return &Components{
		Publisher: NewAsyncPublisher(syncPublisher, publishWorker, cfg.DialTimeout()),
		Consumer:  NewConsumerManager(runtimeCfg),
	}, nil
}

// buildPublishWorkerConfig 将 Kafka 异步发布工作池配置转换为 worker 配置
func buildPublishWorkerConfig(cfg PublishWorkerConfig) worker.WorkConfig {
	workNum := cfg.WorkNum
	if workNum <= 0 {
		workNum = 4
	}
	size := cfg.Size
	if size <= 0 {
		size = 256
	}
	workCfg := worker.WorkConfig{
		Name:    "kafka.publish",
		WorkNum: workNum,
		Size:    size,
	}
	if cfg.SubmitTimeoutSeconds > 0 {
		workCfg.SubmitTimeout = time.Duration(cfg.SubmitTimeoutSeconds) * time.Second
	}
	return workCfg
}

// Close 释放 Kafka 发布者与消费者资源
// Consumer 与 Publisher 为可选依赖（按 InfraOptions 启用），故保留 nil 判空
func (c *Components) Close() error {
	var joined error
	if c.Consumer != nil {
		joined = errors.Join(joined, c.Consumer.Close())
	}
	if c.Publisher != nil {
		joined = errors.Join(joined, c.Publisher.Close())
	}
	return joined
}

// bootstrap 初始化运行时配置并验证 broker 连接可用性
func bootstrap(cfg Config) (KafkaRuntimeConfig, error) {
	runtimeCfg, err := toRuntimeConfig(cfg)
	if err != nil {
		return KafkaRuntimeConfig{}, err
	}

	ctx, cancel := appctx.BackgroundWithTimeout(cfg.DialTimeout())
	defer cancel()

	if err := validateBrokers(ctx, runtimeCfg.Dialer, runtimeCfg.Brokers); err != nil {
		return KafkaRuntimeConfig{}, errors.Wrap(err, "kafka连接验证失败")
	}

	return runtimeCfg, nil
}

// buildWriter 根据配置创建 Kafka 生产者 Writer
func buildWriter(cfg Config, brokers []string) (*kafkago.Writer, error) {
	writer := &kafkago.Writer{
		Addr:         kafkago.TCP(brokers...),
		Balancer:     &kafkago.LeastBytes{},
		RequiredAcks: requiredAcksFromConfig(cfg.RequiredAcks),
	}
	if cfg.BatchSize > 0 {
		writer.BatchSize = cfg.BatchSize
	}
	if batchTimeout := cfg.BatchTimeout(); batchTimeout > 0 {
		writer.BatchTimeout = batchTimeout
	}
	transport, err := buildWriterTransport(cfg)
	if err != nil {
		return nil, err
	}
	if transport != nil {
		writer.Transport = transport
	}
	return writer, nil
}

// effectiveBatchSize 返回实际生效的生产者批量条数
func effectiveBatchSize(batchSize int) int {
	if batchSize > 0 {
		return batchSize
	}
	return 100
}

// effectiveBatchTimeoutSeconds 返回实际生效的生产者批量超时秒数
func effectiveBatchTimeoutSeconds(batchTimeoutSeconds int) int {
	if batchTimeoutSeconds > 0 {
		return batchTimeoutSeconds
	}
	return 1
}

// toRuntimeConfig 将应用配置转换为 Kafka 运行时配置
func toRuntimeConfig(cfg Config) (KafkaRuntimeConfig, error) {
	brokers := make([]string, 0, len(cfg.Brokers))
	for _, broker := range cfg.Brokers {
		trimmed := strings.TrimSpace(broker)
		if trimmed != "" {
			brokers = append(brokers, trimmed)
		}
	}
	startOffset := kafkago.FirstOffset
	if cfg.StartOffset == "latest" {
		startOffset = kafkago.LastOffset
	}

	dialer, err := buildDialer(cfg)
	if err != nil {
		return KafkaRuntimeConfig{}, err
	}

	return KafkaRuntimeConfig{
		Brokers:     brokers,
		ClientID:    strings.TrimSpace(cfg.ClientID),
		MinBytes:    cfg.MinBytes,
		MaxBytes:    cfg.MaxBytes,
		MaxWait:     cfg.MaxWait(),
		StartOffset: startOffset,
		Dialer:      dialer,
	}, nil
}

// validateBrokers 逐个拨号验证 broker 列表可达
func validateBrokers(ctx context.Context, dialer *kafkago.Dialer, brokers []string) error {
	if dialer == nil {
		return errors.New("kafka dialer 未初始化")
	}

	var joined error
	for _, broker := range brokers {
		conn, err := dialer.DialContext(ctx, "tcp", broker)
		if err != nil {
			joined = errors.Join(joined, errors.Wrap(err, "连接 broker %s 失败", broker))
			continue
		}
		_ = conn.Close()
	}
	if joined != nil {
		return joined
	}
	return nil
}

// buildReaderConfig 根据运行时配置与消费参数构建 Reader 配置
func buildReaderConfig(runtimeCfg KafkaRuntimeConfig, cfg ConsumerConfig) (kafkago.ReaderConfig, error) {
	if err := validateConsumerConfig(cfg); err != nil {
		return kafkago.ReaderConfig{}, err
	}

	readerCfg := kafkago.ReaderConfig{
		Brokers:     runtimeCfg.Brokers,
		GroupID:     cfg.GroupID,
		GroupTopics: cfg.Topics,
		MinBytes:    firstPositive(cfg.MinBytes, runtimeCfg.MinBytes, 1),
		MaxBytes:    firstPositive(cfg.MaxBytes, runtimeCfg.MaxBytes, 10e6),
		MaxWait:     firstDuration(cfg.MaxWait, runtimeCfg.MaxWait, time.Second),
		StartOffset: firstOffset(cfg.StartOffset, runtimeCfg.StartOffset),
		Dialer:      runtimeCfg.Dialer,
	}

	return readerCfg, nil
}

// validateConsumerConfig 校验消费者订阅参数
func validateConsumerConfig(cfg ConsumerConfig) error {
	if len(cfg.Topics) == 0 {
		return errors.New("kafka 消费主题不能为空")
	}
	if strings.TrimSpace(cfg.GroupID) == "" {
		return errors.New("kafka 消费组不能为空")
	}
	if _, err := effectiveMaxHandlerRetries(cfg); err != nil {
		return err
	}
	return nil
}

// effectiveMaxHandlerRetries 返回生效的 handler 最大重试次数，未设置时默认 -1（无限重试）
func effectiveMaxHandlerRetries(cfg ConsumerConfig) (int, error) {
	retries := -1
	if cfg.MaxHandlerRetries != nil {
		retries = *cfg.MaxHandlerRetries
	}
	if retries < -1 {
		return 0, errors.New("kafka 消费 handler 最大重试次数不能小于 -1")
	}
	return retries, nil
}

// requiredAcksFromConfig 将配置值映射为 kafka-go 生产者确认级别
func requiredAcksFromConfig(value int) kafkago.RequiredAcks {
	switch value {
	case 0:
		return kafkago.RequireNone
	case 1:
		return kafkago.RequireOne
	default:
		return kafkago.RequireAll
	}
}

// firstPositive 返回第一个大于 0 的整数值，均无效时返回 0
func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

// firstDuration 返回第一个大于 0 的时长值，均无效时返回 0
func firstDuration(values ...time.Duration) time.Duration {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

// firstOffset 返回第一个非 0 的偏移量，均无效时返回 kafka.FirstOffset
func firstOffset(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return kafkago.FirstOffset
}
