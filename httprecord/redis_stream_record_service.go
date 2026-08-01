// Package httprecord 提供 HTTP 访问记录采集与处理能力
//
// @author zwei
package httprecord

import (
	"context"

	"gitee.com/hq_project/yomi-boot/logger"
	"gitee.com/hq_project/yomi-boot/redisstream"
)

// RedisStreamHttpRecordService 通过 Redis Stream 异步发布 HTTP 访问记录
type RedisStreamHttpRecordService struct {
	publisher redisstream.PublisherService
	stream    string
}

var _ HttpRecordService = (*RedisStreamHttpRecordService)(nil)

// NewRedisStreamHttpRecordService 创建 Redis Stream HTTP 访问记录服务
// publisher 与 stream 为必填依赖，缺失时启动期 fail-fast
func NewRedisStreamHttpRecordService(publisher redisstream.PublisherService, stream string) *RedisStreamHttpRecordService {
	if publisher == nil {
		panic("httprecord: NewRedisStreamHttpRecordService publisher 不能为 nil")
	}
	if stream == "" {
		panic("httprecord: NewRedisStreamHttpRecordService stream 不能为空")
	}
	return &RedisStreamHttpRecordService{
		publisher: publisher,
		stream:    stream,
	}
}

// Handle 将访问记录异步发布到 Redis Stream（复用发布工作池，尽快返回）
func (s *RedisStreamHttpRecordService) Handle(ctx context.Context, record *HttpRecord) {
	defer func() {
		if r := recover(); r != nil {
			logger.Warn(ctx, "HTTP 访问记录 Redis Stream 发布 panic", "panic", r)
		}
	}()

	if record == nil {
		return
	}

	wire := toWireRecord(record)
	if err := s.publisher.AddJSONAsync(ctx, s.stream, wire); err != nil {
		logger.Warn(ctx, "HTTP 访问记录 Redis Stream 发布失败",
			"error", err,
			"stream", s.stream,
			"traceId", wire.TraceID,
		)
	}
}
