package kafka

import (
	"context"
	"fmt"
	"time"
)

// Header 表示 Kafka 消息头
type Header struct {
	Key   string // Key 是消息头键
	Value []byte // Value 是消息头值
}

// PublishRequest 表示单条消息发布请求
type PublishRequest struct {
	Topic   string   // Topic 是目标主题
	Key     []byte   // Key 是消息键，可为 nil。相同内容的键进入同一分区；nil 键按轮询分散，不固定分区
	Value   []byte   // Value 是消息体
	Headers []Header // Headers 是可选消息头
}

// ConsumerConfig 表示消费者运行参数
type ConsumerConfig struct {
	Topics            []string      // Topics 是订阅主题列表
	GroupID           string        // GroupID 是消费组标识
	MinBytes          int           // MinBytes 是单次拉取最小字节数，0 表示使用默认值
	MaxBytes          int           // MaxBytes 是单次拉取最大字节数，0 表示使用默认值
	MaxWait           time.Duration // MaxWait 是拉取等待时长，0 表示使用默认值
	StartOffset       int64         // StartOffset 是起始偏移量，默认 kafka.FirstOffset
	MaxHandlerRetries *int          // MaxHandlerRetries 是 handler 返回 Retry 时的最大重试次数；nil 默认 -1（无限），0 不重试，N>0 最多重试 N 次
}

// Message 表示一条已消费的 Kafka 消息
type Message struct {
	Topic     string    // Topic 是消息所属主题
	Partition int       // Partition 是消息所属分区
	Offset    int64     // Offset 是消息偏移量
	Key       []byte    // Key 是消息键
	Value     []byte    // Value 是消息体
	Time      time.Time // Time 是消息写入时间
	Headers   []Header  // Headers 是消息头
}

// ConsumeResult 是消费处理的三态结果。
// 成功和丢弃提交偏移量，重试不提交。零值是重试，handler 漏返回时不会误提交。
type ConsumeResult int

// 消费处理结果
const (
	ConsumeRetry ConsumeResult = iota // 可重试失败，不提交偏移量
	ConsumeOK                         // 处理成功，提交偏移量
	ConsumeDrop                       // 不可恢复，提交偏移量后跳过
)

// String 返回结果名称，未知值带上原始数字
func (r ConsumeResult) String() string {
	switch r {
	case ConsumeOK:
		return "ok"
	case ConsumeRetry:
		return "retry"
	case ConsumeDrop:
		return "drop"
	default:
		return fmt.Sprintf("unknown(%d)", int(r))
	}
}

// AckOK 返回处理成功并提交偏移量的结果
func AckOK() ConsumeResult {
	return ConsumeOK
}

// Retry 返回可重试失败且不提交偏移量的结果
func Retry() ConsumeResult {
	return ConsumeRetry
}

// AckDrop 返回不可恢复处理且提交偏移量跳过的结果
func AckDrop() ConsumeResult {
	return ConsumeDrop
}

// ConsumeHandler 消费处理函数，通过 ConsumeResult 返回处理结果
type ConsumeHandler func(ctx context.Context, msg Message) ConsumeResult
