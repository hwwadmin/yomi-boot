// Package kafka 提供 Kafka 消息发布能力
//
// @author zwei
package kafka

import (
	"context"
	"encoding/json"
	"sync"

	"gitee.com/hq_project/yomi-boot/boot/errors"

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

// Publisher 封装 Kafka 生产者
type Publisher struct {
	writer  *kafkago.Writer // writer 是底层 Kafka 生产者
	closeMu sync.Mutex      // closeMu 保护 closeOnce 并发调用
	closed  bool            // closed 标记是否已关闭
}

var _ PublisherService = (*Publisher)(nil)

// NewPublisher 创建 Kafka 生产者
// writer 为必填依赖，缺失时启动期 fail-fast
func NewPublisher(writer *kafkago.Writer) PublisherService {
	if writer == nil {
		panic("kafka: NewPublisher writer 不能为 nil")
	}
	return &Publisher{writer: writer}
}

// Publish 向指定主题发布一条消息
func (p *Publisher) Publish(ctx context.Context, req PublishRequest) error {
	return p.PublishBatch(ctx, []PublishRequest{req})
}

// PublishBatch 批量发布多条消息
func (p *Publisher) PublishBatch(ctx context.Context, reqs []PublishRequest) error {
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
func (p *Publisher) PublishJSON(ctx context.Context, topic string, key string, value any) error {
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
func (p *Publisher) PublishAsync(ctx context.Context, req PublishRequest) error {
	return p.Publish(ctx, req)
}

// PublishBatchAsync 异步入队批量发布；无工作池时退化为同步发布
func (p *Publisher) PublishBatchAsync(ctx context.Context, reqs []PublishRequest) error {
	return p.PublishBatch(ctx, reqs)
}

// PublishJSONAsync 序列化 JSON 后异步入队发布；无工作池时退化为同步发布
func (p *Publisher) PublishJSONAsync(ctx context.Context, topic string, key string, value any) error {
	return p.PublishJSON(ctx, topic, key, value)
}

// Close 释放生产者连接，幂等安全
func (p *Publisher) Close() error {
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
