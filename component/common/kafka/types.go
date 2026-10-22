package kafka

import (
	"context"
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
	Key     []byte   // Key 是消息键，可为空
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

// ConsumeStatus 表示消息处理结果
type ConsumeStatus int

// 消息处理结果状态码
const (
	ConsumeStatusOK    ConsumeStatus = iota // 处理成功
	ConsumeStatusRetry                      // 可重试失败，不提交偏移量
	ConsumeStatusDrop                       // 不可恢复，提交偏移量后继续
)

// ConsumeResult 表示消费处理结果，由 handler 显式决定是否提交偏移量
type ConsumeResult struct {
	Status ConsumeStatus // Status 是处理结果
	Ack    bool          // Ack 为 true 时提交当前消息偏移量
}

// AckOK 返回处理成功并提交偏移量的结果
func AckOK() ConsumeResult {
	return ConsumeResult{Status: ConsumeStatusOK, Ack: true}
}

// Retry 返回可重试失败且不提交偏移量的结果
func Retry() ConsumeResult {
	return ConsumeResult{Status: ConsumeStatusRetry, Ack: false}
}

// AckDrop 返回不可恢复处理且提交偏移量跳过的结果
func AckDrop() ConsumeResult {
	return ConsumeResult{Status: ConsumeStatusDrop, Ack: true}
}

// ConsumeHandler 消费处理函数，通过 ConsumeResult 返回处理结果与 ack 决策
type ConsumeHandler func(ctx context.Context, msg Message) ConsumeResult
