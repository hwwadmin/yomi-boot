// Package httprecord 提供 HTTP 访问记录采集配置
//
// @author zwei
package httprecord

import (
	"strings"

	"gitee.com/hq_project/yomi-boot/config"
	"gitee.com/hq_project/yomi-boot/errors"
)

// HTTP 访问记录默认值
const (
	defaultHTTPRecordMaxRequestBodySize  = 4096
	defaultHTTPRecordMaxResponseBodySize = 4096
	defaultHttpRecordKafkaTopic          = "yomi.http.record"
	defaultHttpRecordConsumerGroupID     = "httprecord-service"
)

// SectionKey YAML 根 key
const SectionKey = "http_record"

// ConsumerConfig 表示 HTTP 访问记录 Kafka 消费者配置
type ConsumerConfig struct {
	Enabled *bool  `mapstructure:"enabled"`  // Enabled 是否启用 Kafka 消费者，未配置默认 false
	GroupID string `mapstructure:"group_id"` // GroupID 是消费组标识
}

// EnabledOrDefault 返回是否启用 Kafka 消费者，未配置默认 false
func (c ConsumerConfig) EnabledOrDefault() bool {
	return config.BoolOrDefault(c.Enabled)
}

// Config 表示 HTTP 访问记录采集配置（Kafka 审计入库；默认关闭，与文件 access 日志无关）
type Config struct {
	Enabled             *bool          `mapstructure:"enabled"`                // Enabled 是否启用 HTTP 访问记录，未配置默认 false
	KafkaTopic          string         `mapstructure:"kafka_topic"`            // KafkaTopic 是 Kafka 发布/订阅主题
	SkipPaths           []string       `mapstructure:"skip_paths"`             // SkipPaths 是不进行记录采集的接口路径列表
	MaxRequestBodySize  int            `mapstructure:"max_request_body_size"`  // MaxRequestBodySize 是请求体记录最大字节数
	MaxResponseBodySize int            `mapstructure:"max_response_body_size"` // MaxResponseBodySize 是响应体记录最大字节数
	RecordRequestBody   bool           `mapstructure:"record_request_body"`    // RecordRequestBody 是否记录请求体
	RecordResponseBody  bool           `mapstructure:"record_response_body"`   // RecordResponseBody 是否记录响应体
	MaskSensitive       bool           `mapstructure:"mask_sensitive"`         // MaskSensitive 是否对敏感字段脱敏
	Consumer            ConsumerConfig `mapstructure:"consumer"`               // Consumer 是 Kafka 消费者配置
}

// EnabledOrDefault 返回是否启用 HTTP 访问记录，未配置默认 false
func (c Config) EnabledOrDefault() bool {
	return config.BoolOrDefault(c.Enabled)
}

// Validate 校验 HTTP 访问记录配置本段语义
func (c Config) Validate() error {
	for _, skipPath := range c.SkipPaths {
		if skipPath != strings.TrimSpace(skipPath) {
			return errors.New("http_record.skip_paths 不能含首尾空格")
		}
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

// GroupIDOrDefault 返回消费组标识
func (c ConsumerConfig) GroupIDOrDefault() string {
	if groupID := strings.TrimSpace(c.GroupID); groupID != "" {
		return groupID
	}
	return defaultHttpRecordConsumerGroupID
}

// MaxRequestBodySizeOrDefault 返回请求体记录最大字节数
func (c Config) MaxRequestBodySizeOrDefault() int {
	if c.MaxRequestBodySize <= 0 {
		return defaultHTTPRecordMaxRequestBodySize
	}
	return c.MaxRequestBodySize
}

// MaxResponseBodySizeOrDefault 返回响应体记录最大字节数
func (c Config) MaxResponseBodySizeOrDefault() int {
	if c.MaxResponseBodySize <= 0 {
		return defaultHTTPRecordMaxResponseBodySize
	}
	return c.MaxResponseBodySize
}

// ConfigSection 返回 http_record 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &Config{}, func(c *Config, _ *config.Document) error {
		return c.Validate()
	})
}
