// Package kafka 提供 Kafka 访问配置
//
// @author zwei
package kafka

import (
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/core/config"
	"gitee.com/hq_project/yomi-boot/core/errors"
)

// SectionKey YAML 根 key
const SectionKey = "kafka"

// PublishWorkerConfig 表示 Kafka 异步发布专用工作池配置
type PublishWorkerConfig struct {
	WorkNum              int `mapstructure:"work_num"`               // WorkNum 是并发 worker 数，0 表示使用默认值
	Size                 int `mapstructure:"size"`                   // Size 是任务队列容量，0 表示使用默认值
	SubmitTimeoutSeconds int `mapstructure:"submit_timeout_seconds"` // SubmitTimeoutSeconds 是入队等待超时秒数，0 表示使用 worker 默认值
}

// Config 表示 Kafka 访问配置
type Config struct {
	Enabled             *bool               `mapstructure:"enabled"`               // Enabled 是否启用 Kafka，未配置默认 false
	Brokers             []string            `mapstructure:"brokers"`               // Brokers 是 Kafka broker 地址列表
	ClientID            string              `mapstructure:"client_id"`             // ClientID 是客户端标识
	DialTimeoutSeconds  int                 `mapstructure:"dial_timeout_seconds"`  // DialTimeoutSeconds 是连接超时秒数
	RequiredAcks        *int                `mapstructure:"required_acks"`         // RequiredAcks 是生产者确认级别，-1/0/1；nil 默认 -1（acks=all），显式 0 才表示 RequireNone
	BatchSize           int                 `mapstructure:"batch_size"`            // BatchSize 是生产者批量发送条数，0 表示使用 kafka-go 默认值
	BatchTimeoutSeconds int                 `mapstructure:"batch_timeout_seconds"` // BatchTimeoutSeconds 是生产者批量发送超时秒数，0 表示使用 kafka-go 默认值
	MinBytes            int                 `mapstructure:"min_bytes"`             // MinBytes 是消费者单次拉取最小字节数
	MaxBytes            int                 `mapstructure:"max_bytes"`             // MaxBytes 是消费者单次拉取最大字节数
	MaxWaitSeconds      int                 `mapstructure:"max_wait_seconds"`      // MaxWaitSeconds 是消费者拉取等待秒数
	StartOffset         string              `mapstructure:"start_offset"`          // StartOffset 是消费起始偏移量，earliest 或 latest
	SecurityProtocol    string              `mapstructure:"security_protocol"`     // SecurityProtocol 是安全协议，如 PLAINTEXT、SSL、SASL_PLAINTEXT、SASL_SSL
	SASLMechanism       string              `mapstructure:"sasl_mechanism"`        // SASLMechanism 是 SASL 机制，如 PLAIN、SCRAM-SHA-256、SCRAM-SHA-512
	Username            string              `mapstructure:"username"`              // Username 是 SASL 用户名
	Password            string              `mapstructure:"password"`              // Password 是 SASL 密码
	TLSSkipVerify       bool                `mapstructure:"tls_skip_verify"`       // TLSSkipVerify 是否跳过 TLS 服务端证书校验
	TLSCAFile           string              `mapstructure:"tls_ca_file"`           // TLSCAFile 是 TLS CA 证书文件路径
	PublishWorker       PublishWorkerConfig `mapstructure:"publish_worker"`        // PublishWorker 是异步发布工作池配置
}

// EnabledOrDefault 返回是否启用 Kafka，未配置默认 false
func (c Config) EnabledOrDefault() bool {
	return config.BoolOrDefault(c.Enabled)
}

// RequiredAcksOrDefault 返回生产者确认级别，未配置默认 -1（acks=all）
func (c Config) RequiredAcksOrDefault() int {
	if c.RequiredAcks == nil {
		return -1
	}
	return *c.RequiredAcks
}

// Validate 校验 Kafka 配置语义（仅 enabled=true 时校验）
func (c Config) Validate() error {
	if !c.EnabledOrDefault() {
		return nil
	}
	if len(c.Brokers) == 0 {
		return errors.New("kafka.brokers 不能为空")
	}
	for _, broker := range c.Brokers {
		if strings.TrimSpace(broker) == "" {
			return errors.New("kafka.brokers 不能包含空地址")
		}
	}
	if c.DialTimeoutSeconds <= 0 {
		return errors.New("kafka.dial_timeout_seconds 必须大于 0")
	}
	requiredAcks := c.RequiredAcksOrDefault()
	if requiredAcks != -1 && requiredAcks != 0 && requiredAcks != 1 {
		return errors.New("kafka.required_acks 仅支持 -1、0、1")
	}
	if c.BatchSize < 0 {
		return errors.New("kafka.batch_size 不能小于 0")
	}
	if c.BatchTimeoutSeconds < 0 {
		return errors.New("kafka.batch_timeout_seconds 不能小于 0")
	}
	if c.MinBytes < 0 {
		return errors.New("kafka.min_bytes 不能小于 0")
	}
	if c.MaxBytes < 0 {
		return errors.New("kafka.max_bytes 不能小于 0")
	}
	if c.MaxWaitSeconds < 0 {
		return errors.New("kafka.max_wait_seconds 不能小于 0")
	}
	if startOffset := strings.TrimSpace(c.StartOffset); startOffset != "" &&
		startOffset != "earliest" && startOffset != "latest" {
		return errors.New("kafka.start_offset 仅支持 earliest 或 latest")
	}
	if err := c.PublishWorker.Validate(); err != nil {
		return err
	}
	return validateKafkaSecurity(c)
}

// Validate 校验 Kafka 异步发布工作池配置
func (c PublishWorkerConfig) Validate() error {
	if c.WorkNum < 0 {
		return errors.New("kafka.publish_worker.work_num 不能小于 0")
	}
	if c.Size < 0 {
		return errors.New("kafka.publish_worker.size 不能小于 0")
	}
	if c.SubmitTimeoutSeconds < 0 {
		return errors.New("kafka.publish_worker.submit_timeout_seconds 不能小于 0")
	}
	return nil
}

// DialTimeout 返回 Kafka 连接超时
func (c Config) DialTimeout() time.Duration {
	return time.Duration(c.DialTimeoutSeconds) * time.Second
}

// BatchTimeout 返回 Kafka 生产者批量发送超时，0 表示使用 kafka-go 默认值
func (c Config) BatchTimeout() time.Duration {
	if c.BatchTimeoutSeconds <= 0 {
		return 0
	}
	return time.Duration(c.BatchTimeoutSeconds) * time.Second
}

// MaxWait 返回 Kafka 消费者拉取等待时长
func (c Config) MaxWait() time.Duration {
	return time.Duration(c.MaxWaitSeconds) * time.Second
}

// NormalizeSecurityProtocol 规范化安全协议配置值（大写+去空白，空值时默认 PLAINTEXT）
func NormalizeSecurityProtocol(value string) string {
	protocol := strings.ToUpper(strings.TrimSpace(value))
	if protocol == "" {
		return "PLAINTEXT"
	}
	return protocol
}

// NormalizeSASLMechanism 规范化 SASL 机制字符串（大写+去空白+下划线转连字符）
func NormalizeSASLMechanism(value string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(value), "_", "-"))
}

// validateKafkaSecurity 校验 Kafka 安全协议配置
func validateKafkaSecurity(cfg Config) error {
	protocol := NormalizeSecurityProtocol(cfg.SecurityProtocol)
	switch protocol {
	case "PLAINTEXT", "SSL":
		return nil
	case "SASL_PLAINTEXT", "SASL_SSL":
		return validateKafkaSASLCredentials(cfg)
	default:
		return errors.Newf("kafka.security_protocol 不支持: %s", cfg.SecurityProtocol)
	}
}

// validateKafkaSASLCredentials 校验 Kafka SASL 用户名与密码
func validateKafkaSASLCredentials(cfg Config) error {
	if err := validateKafkaSASLMechanism(cfg.SASLMechanism); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.Username) == "" {
		return errors.New("kafka.username 不能为空")
	}
	if strings.TrimSpace(cfg.Password) == "" {
		return errors.New("kafka.password 不能为空")
	}
	return nil
}

// validateKafkaSASLMechanism 校验 Kafka SASL 机制
func validateKafkaSASLMechanism(mechanism string) error {
	switch NormalizeSASLMechanism(mechanism) {
	case "", "PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512":
		return nil
	default:
		return errors.Newf("kafka.sasl_mechanism 不支持: %s", mechanism)
	}
}

// ConfigSection 返回 kafka 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &Config{}, func(c *Config, _ *config.Document) error {
		return c.Validate()
	})
}
