// Package httprecord 提供 HTTP 访问记录采集与处理能力
//
// @author zwei
package httprecord

import (
	"context"
	"encoding/json"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/logger"

	yomikafka "gitee.com/hq_project/yomi-boot/component/common/kafka"
)

// KafkaConsumerID 是 HTTP 访问记录 Kafka 消费者在 ConsumerManager 中的标识
const KafkaConsumerID = "httprecord"

// KafkaRecordConsumerConfig 表示 HTTP 访问记录 Kafka 消费者参数
type KafkaRecordConsumerConfig struct {
	Topics     []string // Topics 是订阅主题列表
	GroupID    string   // GroupID 是消费组标识
	MaxRetries *int     // MaxRetries 是同一消息 handler 返回 Retry 时的最大重试次数，超限后丢弃；nil 使用默认值，0 表示不重试
}

// MaxRetriesOrDefault 返回最大重试次数，未配置时使用默认值
func (c KafkaRecordConsumerConfig) MaxRetriesOrDefault() int {
	if c.MaxRetries == nil {
		return defaultKafkaRecordMaxRetries
	}
	return *c.MaxRetries
}

// StartKafkaRecordConsumer 启动 HTTP 访问记录 Kafka 消费者
func StartKafkaRecordConsumer(
	parentCtx context.Context,
	consumer yomikafka.ConsumerService,
	id string,
	cfg KafkaRecordConsumerConfig,
	handler RecordHandler,
) error {
	if consumer == nil {
		return errors.New("kafka 消费者未初始化")
	}
	if handler == nil {
		return errors.New("HTTP 访问记录消费处理函数不能为空")
	}
	if len(cfg.Topics) == 0 {
		return errors.New("HTTP 访问记录消费主题不能为空")
	}
	if strings.TrimSpace(cfg.GroupID) == "" {
		return errors.New("HTTP 访问记录消费组不能为空")
	}
	maxRetries := cfg.MaxRetriesOrDefault()
	if maxRetries < 0 {
		return errors.New("HTTP 访问记录消费最大重试次数不能小于 0")
	}

	return consumer.Start(parentCtx, id, yomikafka.ConsumerConfig{
		Topics:            cfg.Topics,
		GroupID:           cfg.GroupID,
		MaxHandlerRetries: &maxRetries,
	}, func(ctx context.Context, msg yomikafka.Message) yomikafka.ConsumeResult {
		var wire wireRecord
		if err := json.Unmarshal(msg.Value, &wire); err != nil {
			logger.Warn(ctx, "HTTP 访问记录消息反序列化失败，已丢弃",
				"error", err,
				"topic", msg.Topic,
				"partition", msg.Partition,
				"offset", msg.Offset,
			)
			return yomikafka.AckDrop()
		}

		record := fromWireRecord(wire)
		if record == nil {
			logger.Warn(ctx, "HTTP 访问记录消息内容无效，已丢弃",
				"topic", msg.Topic,
				"partition", msg.Partition,
				"offset", msg.Offset,
			)
			return yomikafka.AckDrop()
		}

		return mapRecordHandlerResultToKafka(handler(ctx, record))
	})
}

// mapRecordHandlerResultToKafka 将共用 RecordHandleResult 映射为 Kafka ConsumeResult
func mapRecordHandlerResultToKafka(result RecordHandleResult) yomikafka.ConsumeResult {
	switch result.Status {
	case RecordHandleOK:
		return yomikafka.AckOK()
	case RecordHandleRetry:
		return yomikafka.Retry()
	case RecordHandleDrop:
		return yomikafka.AckDrop()
	default:
		return yomikafka.AckDrop()
	}
}
