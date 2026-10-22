package kafkaplugin

import (
	yomikafka "gitee.com/hq_project/yomi-boot/component/common/kafka"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// SectionKey YAML 根 key
const SectionKey = "kafka"

// PublishWorkerBootConfig 表示 Kafka 异步发布专用工作池的 YAML 绑定
type PublishWorkerBootConfig struct {
	WorkNum              int `mapstructure:"work_num"`               // WorkNum 是并发 worker 数，0 表示使用默认值
	Size                 int `mapstructure:"size"`                   // Size 是任务队列容量，0 表示使用默认值
	SubmitTimeoutSeconds int `mapstructure:"submit_timeout_seconds"` // SubmitTimeoutSeconds 是入队等待超时秒数，0 表示使用 worker 默认值
}

// BootConfig 表示 kafka 配置段的 YAML 绑定
type BootConfig struct {
	Enabled             *bool                   `mapstructure:"enabled"`               // Enabled 是否启用 Kafka，未配置默认 false
	Brokers             []string                `mapstructure:"brokers"`               // Brokers 是 Kafka broker 地址列表
	ClientID            string                  `mapstructure:"client_id"`             // ClientID 是客户端标识
	DialTimeoutSeconds  int                     `mapstructure:"dial_timeout_seconds"`  // DialTimeoutSeconds 是连接超时秒数
	RequiredAcks        *int                    `mapstructure:"required_acks"`         // RequiredAcks 是生产者确认级别，-1/0/1；nil 默认 -1（acks=all），显式 0 才表示 RequireNone
	BatchSize           int                     `mapstructure:"batch_size"`            // BatchSize 是生产者批量发送条数，0 表示使用 kafka-go 默认值
	BatchTimeoutSeconds int                     `mapstructure:"batch_timeout_seconds"` // BatchTimeoutSeconds 是生产者批量发送超时秒数，0 表示使用 kafka-go 默认值
	MinBytes            int                     `mapstructure:"min_bytes"`             // MinBytes 是消费者单次拉取最小字节数
	MaxBytes            int                     `mapstructure:"max_bytes"`             // MaxBytes 是消费者单次拉取最大字节数
	MaxWaitSeconds      int                     `mapstructure:"max_wait_seconds"`      // MaxWaitSeconds 是消费者拉取等待秒数
	StartOffset         string                  `mapstructure:"start_offset"`          // StartOffset 是消费起始偏移量，earliest 或 latest
	SecurityProtocol    string                  `mapstructure:"security_protocol"`     // SecurityProtocol 是安全协议，如 PLAINTEXT、SSL、SASL_PLAINTEXT、SASL_SSL
	SASLMechanism       string                  `mapstructure:"sasl_mechanism"`        // SASLMechanism 是 SASL 机制，如 PLAIN、SCRAM-SHA-256、SCRAM-SHA-512
	Username            string                  `mapstructure:"username"`              // Username 是 SASL 用户名
	Password            string                  `mapstructure:"password"`              // Password 是 SASL 密码
	TLSSkipVerify       bool                    `mapstructure:"tls_skip_verify"`       // TLSSkipVerify 是否跳过 TLS 服务端证书校验
	TLSCAFile           string                  `mapstructure:"tls_ca_file"`           // TLSCAFile 是 TLS CA 证书文件路径
	PublishWorker       PublishWorkerBootConfig `mapstructure:"publish_worker"`        // PublishWorker 是异步发布工作池配置
}

// EnabledOrDefault 返回是否启用 Kafka，未配置默认 false
func (c BootConfig) EnabledOrDefault() bool {
	return c.Enabled != nil && *c.Enabled
}

// ToConfig 转换为运行时配置
func (c BootConfig) ToConfig() yomikafka.Config {
	return yomikafka.Config{
		Brokers:             c.Brokers,
		ClientID:            c.ClientID,
		DialTimeoutSeconds:  c.DialTimeoutSeconds,
		RequiredAcks:        c.RequiredAcks,
		BatchSize:           c.BatchSize,
		BatchTimeoutSeconds: c.BatchTimeoutSeconds,
		MinBytes:            c.MinBytes,
		MaxBytes:            c.MaxBytes,
		MaxWaitSeconds:      c.MaxWaitSeconds,
		StartOffset:         c.StartOffset,
		SecurityProtocol:    c.SecurityProtocol,
		SASLMechanism:       c.SASLMechanism,
		Username:            c.Username,
		Password:            c.Password,
		TLSSkipVerify:       c.TLSSkipVerify,
		TLSCAFile:           c.TLSCAFile,
		PublishWorker: yomikafka.PublishWorkerConfig{
			WorkNum:              c.PublishWorker.WorkNum,
			Size:                 c.PublishWorker.Size,
			SubmitTimeoutSeconds: c.PublishWorker.SubmitTimeoutSeconds,
		},
	}
}

// Validate 校验 kafka 配置段语义（仅 enabled=true 时校验）
func (c BootConfig) Validate() error {
	if !c.EnabledOrDefault() {
		return nil
	}
	if err := c.ToConfig().Validate(); err != nil {
		return errors.Wrap(err, "%s", SectionKey)
	}
	return nil
}

// ConfigSection 返回 kafka 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &BootConfig{}, func(c *BootConfig, _ *config.Config) error {
		return c.Validate()
	})
}
