// Package event 提供普通消息事件消费能力
//
// @author zwei
package event

import (
	"context"
	"gitee.com/hq_project/yomi-boot/logger"

	yomikafka "gitee.com/hq_project/yomi-boot/kafka"
)

// NormalReceiver 普通消息事件接收器，允许重复消费与消息丢失
type NormalReceiver struct{}

// NewNormalReceiver 创建普通消息事件接收器
func NewNormalReceiver() *NormalReceiver {
	return &NormalReceiver{}
}

// WrapHandler 将业务处理函数包装为 Kafka 消费 handler，最终总会 ack
func (r *NormalReceiver) WrapHandler(fn func(ctx context.Context, evt Event) error) yomikafka.ConsumeHandler {
	if fn == nil {
		return func(ctx context.Context, msg yomikafka.Message) yomikafka.ConsumeResult {
			logger.Warn(ctx, "消息队列事件 action 未配置")
			return yomikafka.AckDrop()
		}
	}

	return func(ctx context.Context, msg yomikafka.Message) yomikafka.ConsumeResult {
		if len(msg.Value) == 0 {
			logger.Warn(ctx, "消息队列事件为空，已被抛弃")
			return yomikafka.AckDrop()
		}

		evt, ok := ParseAndValidate(msg.Value, false)
		if !ok {
			return yomikafka.AckDrop()
		}
		if evt.EventID != "" {
			logger.Warn(ctx, "NormalReceiver 检测到带 eventId 的事件，如需业务幂等保障请改用 Receiver",
				"eventId", evt.EventID, "topic", evt.Topic)
		}

		if err := fn(ctx, evt); err != nil {
			logger.Error(ctx, "消息队列事件消费异常", "message", string(msg.Value), "error", err)
		}
		return yomikafka.AckOK()
	}
}
