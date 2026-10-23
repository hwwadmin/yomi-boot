// Package httprecord 提供 HTTP 访问记录采集与处理能力
//
// @author zwei
package httprecord

import (
	"context"
	"encoding/json"

	"gitee.com/hq_project/yomi-boot/component/core/logger"

	yomikafka "gitee.com/hq_project/yomi-boot/component/common/kafka"
)

// KafkaHttpRecordService 通过 Kafka 异步发布 HTTP 访问记录
type KafkaHttpRecordService struct {
	publisher yomikafka.PublisherService
	topic     string
}

var _ HttpRecordService = (*KafkaHttpRecordService)(nil)

// NewKafkaHttpRecordService 创建 Kafka HTTP 访问记录服务
// publisher 与 topic 为必填依赖，缺失时启动期 fail-fast
func NewKafkaHttpRecordService(publisher yomikafka.PublisherService, topic string) *KafkaHttpRecordService {
	if publisher == nil {
		panic("httprecord: NewKafkaHttpRecordService publisher 不能为 nil")
	}
	if topic == "" {
		panic("httprecord: NewKafkaHttpRecordService topic 不能为空")
	}
	return &KafkaHttpRecordService{
		publisher: publisher,
		topic:     topic,
	}
}

// Handle 将访问记录异步发布到 Kafka
func (s *KafkaHttpRecordService) Handle(ctx context.Context, record *HttpRecord) {
	defer func() {
		if r := recover(); r != nil {
			logger.Warn(ctx, "HTTP 访问记录 Kafka 发布 panic", "panic", r)
		}
	}()

	// record 为参数校验
	if record == nil {
		return
	}

	payload, err := json.Marshal(toWireRecord(record))
	if err != nil {
		logger.Warn(ctx, "HTTP 访问记录序列化失败", "error", err)
		return
	}

	req := yomikafka.PublishRequest{
		Topic: s.topic,
		Value: payload,
	}
	if record.TraceID != "" {
		req.Key = []byte(record.TraceID)
	}

	if err := s.publisher.PublishAsync(ctx, req); err != nil {
		logger.Warn(ctx, "HTTP 访问记录 Kafka 发布失败", "error", err, "topic", s.topic)
	}
}
