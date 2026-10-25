// Package httprecord 提供 HTTP 访问记录采集配置
//
// @author zwei
package httprecord

import (
	"strings"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/utils/utf8util"
)

// HTTP 访问记录默认值
const (
	defaultHTTPRecordMaxRequestBodySize  = 4096
	defaultHTTPRecordMaxResponseBodySize = 4096
	defaultHttpRecordKafkaTopic          = "yomi.http.record"
	defaultHttpRecordStream              = "yomi.http.record"
	defaultHttpRecordConsumerGroupID     = "httprecord-service"
	defaultHttpRecordProvider            = ProviderMemory
	defaultMemoryQueueSize               = 1024
	defaultMemoryWorkers                 = 2
)

// HttpRecordService 实现提供商标识
const (
	ProviderKafka       = "kafka"        // ProviderKafka 通过 Kafka 异步发布
	ProviderMemory      = "memory"       // ProviderMemory 通过进程内内存队列异步处理
	ProviderRedisStream = "redis_stream" // ProviderRedisStream 通过 Redis Stream 异步发布
)

// ConsumerConfig 表示 HTTP 访问记录消费者配置（Kafka / Redis Stream 共用 group_id）
type ConsumerConfig struct {
	Enabled *bool  // Enabled 是否启用消费者，未配置默认 false
	GroupID string // GroupID 是消费组标识
}

// EnabledOrDefault 返回是否启用消费者，未配置默认 false
func (c ConsumerConfig) EnabledOrDefault() bool {
	return boolOrDefault(c.Enabled)
}

// MemoryConfig 表示进程内内存队列配置（provider=memory 时生效）
type MemoryConfig struct {
	QueueSize int // QueueSize 是内存队列缓冲容量
	Workers   int // Workers 是内存消费协程数（供消费端点读取）
}

// Config 表示 HTTP 访问记录采集配置
type Config struct {
	MiddlewareEnabled   bool           // MiddlewareEnabled 控制采集中间件是否执行；由 BootConfig.enabled 映射
	Provider            string         // Provider 是实现提供商：kafka、memory 或 redis_stream
	KafkaTopic          string         // KafkaTopic 是 Kafka 发布/订阅主题
	Stream              string         // Stream 是 Redis Stream 逻辑名（provider=redis_stream）
	SkipPaths           []string       // SkipPaths 是不进行记录采集的接口路径列表
	MaxRequestBodySize  *int           // MaxRequestBodySize 是请求体记录最大字节数；nil 默认 4096，0 表示不截断
	MaxResponseBodySize *int           // MaxResponseBodySize 是响应体记录最大字节数；nil 默认 4096，0 表示不截断
	RecordRequestBody   bool           // RecordRequestBody 是否记录请求体
	RecordResponseBody  bool           // RecordResponseBody 是否记录响应体
	MaskSensitive       bool           // MaskSensitive 是否用注入的脱敏服务处理记录内容；false 时不调用脱敏组件（组件内聚判定）
	Memory              MemoryConfig   // Memory 是内存队列配置（provider=memory）
	Consumer            ConsumerConfig // Consumer 是消费者配置
}

// boolOrDefault 返回指针布尔值的值，nil 视为 false
func boolOrDefault(v *bool) bool {
	return v != nil && *v
}

// ProviderOrDefault 返回实现提供商，未配置时默认 memory
func (c Config) ProviderOrDefault() string {
	if provider := strings.ToLower(strings.TrimSpace(c.Provider)); provider != "" {
		return provider
	}
	return defaultHttpRecordProvider
}

// Validate 校验 HTTP 访问记录配置语义
func (c Config) Validate() error {
	for _, skipPath := range c.SkipPaths {
		if skipPath != strings.TrimSpace(skipPath) {
			return errors.New("skip_paths 不能含首尾空格")
		}
	}
	if c.MaxRequestBodySize != nil && *c.MaxRequestBodySize < 0 {
		return errors.New("max_request_body_size 不能小于 0")
	}
	if c.MaxResponseBodySize != nil && *c.MaxResponseBodySize < 0 {
		return errors.New("max_response_body_size 不能小于 0")
	}
	switch c.ProviderOrDefault() {
	case ProviderKafka, ProviderMemory, ProviderRedisStream:
	default:
		return errors.Newf("provider 不支持: %s", c.Provider)
	}
	if c.Memory.QueueSize < 0 {
		return errors.New("memory.queue_size 不能小于 0")
	}
	if c.Memory.Workers < 0 {
		return errors.New("memory.workers 不能小于 0")
	}
	return nil
}

// KafkaTopicOrDefault 返回 Kafka 主题，未配置时使用默认值
func (c Config) KafkaTopicOrDefault() string {
	if topic := strings.TrimSpace(c.KafkaTopic); topic != "" {
		return topic
	}
	return defaultHttpRecordKafkaTopic
}

// StreamOrDefault 返回 Redis Stream 逻辑名，未配置时使用默认值
func (c Config) StreamOrDefault() string {
	if stream := strings.TrimSpace(c.Stream); stream != "" {
		return stream
	}
	return defaultHttpRecordStream
}

// GroupIDOrDefault 返回消费组标识
func (c ConsumerConfig) GroupIDOrDefault() string {
	if groupID := strings.TrimSpace(c.GroupID); groupID != "" {
		return groupID
	}
	return defaultHttpRecordConsumerGroupID
}

// QueueSizeOrDefault 返回内存队列缓冲容量
func (c MemoryConfig) QueueSizeOrDefault() int {
	if c.QueueSize <= 0 {
		return defaultMemoryQueueSize
	}
	return c.QueueSize
}

// WorkersOrDefault 返回内存消费协程数
func (c MemoryConfig) WorkersOrDefault() int {
	if c.Workers <= 0 {
		return defaultMemoryWorkers
	}
	return c.Workers
}

// MaxRequestBodySizeOrDefault 返回请求体记录最大字节数；未配置用默认值，显式 0 表示不截断
func (c Config) MaxRequestBodySizeOrDefault() int {
	return resolveBodySizeLimit(c.MaxRequestBodySize, defaultHTTPRecordMaxRequestBodySize)
}

// MaxResponseBodySizeOrDefault 返回响应体记录最大字节数；未配置用默认值，显式 0 表示不截断
func (c Config) MaxResponseBodySizeOrDefault() int {
	return resolveBodySizeLimit(c.MaxResponseBodySize, defaultHTTPRecordMaxResponseBodySize)
}

// resolveBodySizeLimit 将接入方配置解析为采集上限：nil 用默认值，0 为 utf8util.Unlimited
func resolveBodySizeLimit(configured *int, defaultSize int) int {
	if configured == nil {
		return defaultSize
	}
	if *configured == 0 {
		return utf8util.Unlimited
	}
	return *configured
}
