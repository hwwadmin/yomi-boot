// Package httprecord 提供 HTTP 访问记录采集配置
//
// @author zwei
package httprecord

import (
	"strings"

	"gitee.com/hq_project/yomi-boot/core/config"
	"gitee.com/hq_project/yomi-boot/core/errors"
	"gitee.com/hq_project/yomi-boot/core/utils/utf8util"
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

// SectionKey YAML 根 key
const SectionKey = "http_record"

// ConsumerConfig 表示 HTTP 访问记录消费者配置（Kafka / Redis Stream 共用 group_id）
type ConsumerConfig struct {
	Enabled *bool  `mapstructure:"enabled"`  // Enabled 是否启用消费者，未配置默认 false
	GroupID string `mapstructure:"group_id"` // GroupID 是消费组标识
}

// EnabledOrDefault 返回是否启用消费者，未配置默认 false
func (c ConsumerConfig) EnabledOrDefault() bool {
	return config.BoolOrDefault(c.Enabled)
}

// MemoryConfig 表示进程内内存队列配置（provider=memory 时生效）
type MemoryConfig struct {
	QueueSize int `mapstructure:"queue_size"` // QueueSize 是内存队列缓冲容量
	Workers   int `mapstructure:"workers"`    // Workers 是内存消费协程数（供消费端点读取）
}

// Config 表示 HTTP 访问记录采集配置（默认关闭，与文件 access 日志无关）
type Config struct {
	Enabled             *bool          `mapstructure:"enabled"`                // Enabled 是否启用 HTTP 访问记录，未配置默认 false
	Provider            string         `mapstructure:"provider"`               // Provider 是实现提供商：kafka、memory 或 redis_stream
	KafkaTopic          string         `mapstructure:"kafka_topic"`            // KafkaTopic 是 Kafka 发布/订阅主题
	Stream              string         `mapstructure:"stream"`                 // Stream 是 Redis Stream 逻辑名（provider=redis_stream）
	SkipPaths           []string       `mapstructure:"skip_paths"`             // SkipPaths 是不进行记录采集的接口路径列表
	MaxRequestBodySize  *int           `mapstructure:"max_request_body_size"`  // MaxRequestBodySize 是请求体记录最大字节数；nil 默认 4096，0 表示不截断
	MaxResponseBodySize *int           `mapstructure:"max_response_body_size"` // MaxResponseBodySize 是响应体记录最大字节数；nil 默认 4096，0 表示不截断
	RecordRequestBody   bool           `mapstructure:"record_request_body"`    // RecordRequestBody 是否记录请求体
	RecordResponseBody  bool           `mapstructure:"record_response_body"`   // RecordResponseBody 是否记录响应体
	MaskSensitive       bool           `mapstructure:"mask_sensitive"`         // MaskSensitive 是否对敏感字段脱敏
	Memory              MemoryConfig   `mapstructure:"memory"`                 // Memory 是内存队列配置（provider=memory）
	Consumer            ConsumerConfig `mapstructure:"consumer"`               // Consumer 是消费者配置
}

// EnabledOrDefault 返回是否启用 HTTP 访问记录，未配置默认 false
func (c Config) EnabledOrDefault() bool {
	return config.BoolOrDefault(c.Enabled)
}

// ProviderOrDefault 返回实现提供商，未配置时默认 memory
func (c Config) ProviderOrDefault() string {
	if provider := strings.ToLower(strings.TrimSpace(c.Provider)); provider != "" {
		return provider
	}
	return defaultHttpRecordProvider
}

// Validate 校验 HTTP 访问记录配置本段语义
func (c Config) Validate() error {
	for _, skipPath := range c.SkipPaths {
		if skipPath != strings.TrimSpace(skipPath) {
			return errors.New("http_record.skip_paths 不能含首尾空格")
		}
	}
	if c.MaxRequestBodySize != nil && *c.MaxRequestBodySize < 0 {
		return errors.New("http_record.max_request_body_size 不能小于 0")
	}
	if c.MaxResponseBodySize != nil && *c.MaxResponseBodySize < 0 {
		return errors.New("http_record.max_response_body_size 不能小于 0")
	}
	if !c.EnabledOrDefault() {
		return nil
	}
	switch c.ProviderOrDefault() {
	case ProviderKafka, ProviderMemory, ProviderRedisStream:
	default:
		return errors.Newf("http_record.provider 不支持: %s", c.Provider)
	}
	if c.Memory.QueueSize < 0 {
		return errors.New("http_record.memory.queue_size 不能小于 0")
	}
	if c.Memory.Workers < 0 {
		return errors.New("http_record.memory.workers 不能小于 0")
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

// ConfigSection 返回 http_record 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &Config{}, func(c *Config, _ *config.Document) error {
		return c.Validate()
	})
}
