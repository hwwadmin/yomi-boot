package kafka

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/logger"
	"gitee.com/hq_project/yomi-boot/component/common/worker"
	"gitee.com/hq_project/yomi-boot/component/core/errors"

	kafkago "github.com/segmentio/kafka-go"
)

// PublisherService 表示 Kafka 消息发布能力接口
type PublisherService interface {
	// Publish 向指定主题发布一条消息
	Publish(ctx context.Context, req PublishRequest) error
	// PublishBatch 批量发布多条消息
	PublishBatch(ctx context.Context, reqs []PublishRequest) error
	// PublishJSON 将对象序列化为 JSON 后发布到指定主题
	PublishJSON(ctx context.Context, topic string, key string, value any) error
	// PublishAsync 异步入队发布；成功表示任务已进入工作池，不代表 Kafka 已确认
	PublishAsync(ctx context.Context, req PublishRequest) error
	// PublishBatchAsync 异步入队批量发布
	PublishBatchAsync(ctx context.Context, reqs []PublishRequest) error
	// PublishJSONAsync 序列化 JSON 后异步入队发布
	PublishJSONAsync(ctx context.Context, topic string, key string, value any) error
	// Close 释放生产者连接
	Close() error
}

// NewPublisher 根据生产者配置创建 Kafka 生产者；发布工作池随生产者创建并启动
func NewPublisher(cfg PublisherConfig) (PublisherService, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	conn, err := newConnection(cfg.Connection)
	if err != nil {
		return nil, err
	}
	if err := validateConnection(conn, cfg.Connection.DialTimeout()); err != nil {
		return nil, errors.Wrap(err, "kafka连接验证失败")
	}

	writer, err := buildWriter(cfg, conn.brokers)
	if err != nil {
		return nil, err
	}

	syncer := newSyncPublisher(writer)
	publishWorkerCfg := buildPublishWorkerConfig(cfg.PublishWorker)
	publishWorker, err := worker.NewWorker(publishWorkerCfg, context.Background())
	if err != nil {
		_ = syncer.Close()
		return nil, err
	}
	publishWorker.Start()

	logger.InfoWithoutCtx(
		"初始化 Kafka 生产者成功",
		"brokers", strings.Join(conn.brokers, ","),
		"security_protocol", NormalizeSecurityProtocol(cfg.Connection.SecurityProtocol),
		"sasl_mechanism", NormalizeSASLMechanism(cfg.Connection.SASLMechanism),
		"batch_size", effectiveBatchSize(cfg.BatchSize),
		"batch_timeout_seconds", effectiveBatchTimeoutSeconds(cfg.BatchTimeoutSeconds),
		"publish_worker_num", publishWorkerCfg.WorkNum,
		"publish_worker_size", publishWorkerCfg.Size,
	)

	return newAsyncPublisher(syncer, publishWorker, cfg.Connection.DialTimeout()), nil
}

// syncPublisher 封装 Kafka 同步生产者
type syncPublisher struct {
	writer  *kafkago.Writer // writer 是底层 Kafka 生产者
	closeMu sync.Mutex      // closeMu 保护 closed 并发调用
	closed  bool            // closed 标记是否已关闭
}

var _ PublisherService = (*syncPublisher)(nil)

// newSyncPublisher 创建 Kafka 同步生产者
// writer 为必填依赖，缺失时启动期 fail-fast
func newSyncPublisher(writer *kafkago.Writer) PublisherService {
	if writer == nil {
		panic("kafka: newSyncPublisher writer 不能为 nil")
	}
	return &syncPublisher{writer: writer}
}

// Publish 向指定主题发布一条消息
func (p *syncPublisher) Publish(ctx context.Context, req PublishRequest) error {
	return p.PublishBatch(ctx, []PublishRequest{req})
}

// PublishBatch 批量发布多条消息
func (p *syncPublisher) PublishBatch(ctx context.Context, reqs []PublishRequest) error {
	if len(reqs) == 0 {
		return nil
	}

	msgs := make([]kafkago.Message, 0, len(reqs))
	for i, req := range reqs {
		if err := validatePublishRequest(req); err != nil {
			return errors.Wrap(err, "批量消息第 %d 条校验失败", i)
		}
		msgs = append(msgs, toKafkaMessage(req))
	}

	if err := p.writer.WriteMessages(ctx, msgs...); err != nil {
		return errors.Wrap(err, "批量发布 Kafka 消息失败")
	}

	return nil
}

// PublishJSON 将对象序列化为 JSON 后发布到指定主题
func (p *syncPublisher) PublishJSON(ctx context.Context, topic string, key string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return errors.Wrap(err, "序列化 Kafka JSON 消息失败")
	}

	var keyBytes []byte
	if key != "" {
		keyBytes = []byte(key)
	}

	return p.Publish(ctx, PublishRequest{
		Topic: topic,
		Key:   keyBytes,
		Value: payload,
	})
}

// PublishAsync 异步入队发布；无工作池时退化为同步发布
func (p *syncPublisher) PublishAsync(ctx context.Context, req PublishRequest) error {
	return p.Publish(ctx, req)
}

// PublishBatchAsync 异步入队批量发布；无工作池时退化为同步发布
func (p *syncPublisher) PublishBatchAsync(ctx context.Context, reqs []PublishRequest) error {
	return p.PublishBatch(ctx, reqs)
}

// PublishJSONAsync 序列化 JSON 后异步入队发布；无工作池时退化为同步发布
func (p *syncPublisher) PublishJSONAsync(ctx context.Context, topic string, key string, value any) error {
	return p.PublishJSON(ctx, topic, key, value)
}

// Close 释放生产者连接，幂等安全
func (p *syncPublisher) Close() error {
	p.closeMu.Lock()
	defer p.closeMu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	if err := p.writer.Close(); err != nil {
		return errors.Wrap(err, "关闭 Kafka 生产者失败")
	}
	return nil
}

// buildWriter 根据生产者配置创建 Kafka Writer
func buildWriter(cfg PublisherConfig, brokers []string) (*kafkago.Writer, error) {
	writer := &kafkago.Writer{
		Addr:         kafkago.TCP(brokers...),
		Balancer:     &kafkago.LeastBytes{},
		RequiredAcks: requiredAcksFromConfig(cfg.RequiredAcksOrDefault()),
	}
	if cfg.BatchSize > 0 {
		writer.BatchSize = cfg.BatchSize
	}
	if batchTimeout := cfg.BatchTimeout(); batchTimeout > 0 {
		writer.BatchTimeout = batchTimeout
	}
	transport, err := buildWriterTransport(cfg.Connection)
	if err != nil {
		return nil, err
	}
	if transport != nil {
		writer.Transport = transport
	}
	return writer, nil
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

// validatePublishRequest 校验消息发布请求参数
func validatePublishRequest(req PublishRequest) error {
	if req.Topic == "" {
		return errors.New("kafka topic 不能为空")
	}
	if len(req.Value) == 0 {
		return errors.New("kafka 消息体不能为空")
	}
	return nil
}

// toKafkaMessage 将发布请求转换为 kafka-go 消息
func toKafkaMessage(req PublishRequest) kafkago.Message {
	return kafkago.Message{
		Topic:   req.Topic,
		Key:     req.Key,
		Value:   req.Value,
		Headers: toKafkaHeaders(req.Headers),
	}
}

// toKafkaHeaders 将业务消息头转换为 kafka-go 消息头
func toKafkaHeaders(headers []Header) []kafkago.Header {
	if len(headers) == 0 {
		return nil
	}

	result := make([]kafkago.Header, 0, len(headers))
	for _, header := range headers {
		result = append(result, kafkago.Header{
			Key:   header.Key,
			Value: header.Value,
		})
	}
	return result
}
