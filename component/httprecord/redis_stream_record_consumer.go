// Package httprecord 提供 HTTP 访问记录采集与处理能力
//
// @author zwei
package httprecord

import (
	"context"
	"encoding/json"
	"strings"

	"gitee.com/hq_project/yomi-boot/boot/errors"
	"gitee.com/hq_project/yomi-boot/boot/logger"
	"gitee.com/hq_project/yomi-boot/component/redisstream"
)

// RedisStreamConsumerID 是 HTTP 访问记录 Redis Stream 消费者在 ConsumerManager 中的标识
const RedisStreamConsumerID = "httprecord"

// RedisStreamRecordConsumerConfig 表示 HTTP 访问记录 Redis Stream 消费者参数
type RedisStreamRecordConsumerConfig struct {
	Stream   string // Stream 是逻辑 stream 名
	Group    string // Group 是消费组标识
	Consumer string // Consumer 是本实例消费者名（同组内须唯一）
}

// StartRedisStreamRecordConsumer 启动 HTTP 访问记录 Redis Stream 消费者
func StartRedisStreamRecordConsumer(
	parentCtx context.Context,
	consumer redisstream.ConsumerService,
	id string,
	cfg RedisStreamRecordConsumerConfig,
	handler RecordHandler,
) error {
	if consumer == nil {
		return errors.New("redis stream 消费者未初始化")
	}
	if handler == nil {
		return errors.New("HTTP 访问记录消费处理函数不能为空")
	}
	if strings.TrimSpace(cfg.Stream) == "" {
		return errors.New("HTTP 访问记录消费 stream 不能为空")
	}
	if strings.TrimSpace(cfg.Group) == "" {
		return errors.New("HTTP 访问记录消费组不能为空")
	}
	if strings.TrimSpace(cfg.Consumer) == "" {
		return errors.New("HTTP 访问记录消费者名不能为空")
	}

	return consumer.Start(parentCtx, id, redisstream.GroupConfig{
		Stream:   cfg.Stream,
		Group:    cfg.Group,
		Consumer: cfg.Consumer,
	}, func(ctx context.Context, entry redisstream.Entry) redisstream.HandleResult {
		payload := entry.Payload()
		if len(payload) == 0 {
			logger.Warn(ctx, "HTTP 访问记录 Redis Stream 消息无 payload，已丢弃",
				"stream", entry.Stream,
				"id", entry.ID,
			)
			return redisstream.HandleDead
		}

		var wire wireRecord
		if err := json.Unmarshal(payload, &wire); err != nil {
			logger.Warn(ctx, "HTTP 访问记录 Redis Stream 消息反序列化失败，已丢弃",
				"error", err,
				"stream", entry.Stream,
				"id", entry.ID,
			)
			return redisstream.HandleDead
		}

		record := fromWireRecord(wire)
		if record == nil {
			logger.Warn(ctx, "HTTP 访问记录 Redis Stream 消息内容无效，已丢弃",
				"stream", entry.Stream,
				"id", entry.ID,
			)
			return redisstream.HandleDead
		}

		return mapRecordHandlerResultToRedisStream(handler(ctx, record))
	})
}

// mapRecordHandlerResultToRedisStream 将共用 RecordHandleResult 映射为 Redis Stream HandleResult
func mapRecordHandlerResultToRedisStream(result RecordHandleResult) redisstream.HandleResult {
	switch result.Status {
	case RecordHandleOK:
		return redisstream.HandleAck
	case RecordHandleRetry:
		return redisstream.HandleRetry
	case RecordHandleDrop:
		return redisstream.HandleDead
	default:
		return redisstream.HandleDead
	}
}
