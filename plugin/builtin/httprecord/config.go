package httprecordplugin

import (
	"gitee.com/hq_project/yomi-boot/component/common/httprecord"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// SectionKey YAML 根 key
const SectionKey = "http_record"

// ConsumerBootConfig 表示 HTTP 访问记录消费者的 YAML 绑定
type ConsumerBootConfig struct {
	Enabled *bool  `mapstructure:"enabled"`  // Enabled 是否启用消费者，未配置默认 false
	GroupID string `mapstructure:"group_id"` // GroupID 是消费组标识
}

// MemoryBootConfig 表示进程内内存队列的 YAML 绑定
type MemoryBootConfig struct {
	QueueSize int `mapstructure:"queue_size"` // QueueSize 是内存队列缓冲容量
	Workers   int `mapstructure:"workers"`    // Workers 是内存消费协程数（供消费端点读取）
}

// BootConfig 表示 http_record 配置段的 YAML 绑定
type BootConfig struct {
	Enabled             *bool              `mapstructure:"enabled"`                // Enabled 是否启用 HTTP 访问记录，未配置默认 false
	Provider            string             `mapstructure:"provider"`               // Provider 是实现提供商：kafka、memory 或 redis_stream
	KafkaTopic          string             `mapstructure:"kafka_topic"`            // KafkaTopic 是 Kafka 发布/订阅主题
	Stream              string             `mapstructure:"stream"`                 // Stream 是 Redis Stream 逻辑名（provider=redis_stream）
	SkipPaths           []string           `mapstructure:"skip_paths"`             // SkipPaths 是不进行记录采集的接口路径列表
	MaxRequestBodySize  *int               `mapstructure:"max_request_body_size"`  // MaxRequestBodySize 是请求体记录最大字节数；nil 默认 4096，0 表示不截断
	MaxResponseBodySize *int               `mapstructure:"max_response_body_size"` // MaxResponseBodySize 是响应体记录最大字节数；nil 默认 4096，0 表示不截断
	RecordRequestBody   bool               `mapstructure:"record_request_body"`    // RecordRequestBody 是否记录请求体
	RecordResponseBody  bool               `mapstructure:"record_response_body"`   // RecordResponseBody 是否记录响应体
	MaskSensitive       bool               `mapstructure:"mask_sensitive"`         // MaskSensitive 是否对敏感字段脱敏
	Memory              MemoryBootConfig   `mapstructure:"memory"`                 // Memory 是内存队列配置（provider=memory）
	Consumer            ConsumerBootConfig `mapstructure:"consumer"`               // Consumer 是消费者配置
}

// EnabledOrDefault 返回是否启用 HTTP 访问记录，未配置默认 false
func (c BootConfig) EnabledOrDefault() bool {
	return c.Enabled != nil && *c.Enabled
}

// ToConfig 转换为运行时配置
func (c BootConfig) ToConfig() httprecord.Config {
	return httprecord.Config{
		MiddlewareEnabled:   c.EnabledOrDefault(),
		Provider:            c.Provider,
		KafkaTopic:          c.KafkaTopic,
		Stream:              c.Stream,
		SkipPaths:           c.SkipPaths,
		MaxRequestBodySize:  c.MaxRequestBodySize,
		MaxResponseBodySize: c.MaxResponseBodySize,
		RecordRequestBody:   c.RecordRequestBody,
		RecordResponseBody:  c.RecordResponseBody,
		MaskSensitive:       c.MaskSensitive,
		Memory: httprecord.MemoryConfig{
			QueueSize: c.Memory.QueueSize,
			Workers:   c.Memory.Workers,
		},
		Consumer: httprecord.ConsumerConfig{
			Enabled: c.Consumer.Enabled,
			GroupID: c.Consumer.GroupID,
		},
	}
}

// Validate 校验 http_record 配置段语义（仅 enabled=true 时校验）
func (c BootConfig) Validate() error {
	if !c.EnabledOrDefault() {
		return nil
	}
	if err := c.ToConfig().Validate(); err != nil {
		return errors.Wrap(err, "%s", SectionKey)
	}
	return nil
}

// ConfigSection 返回 http_record 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &BootConfig{}, func(c *BootConfig, _ *config.Config) error {
		return c.Validate()
	})
}
