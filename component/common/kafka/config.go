package kafka

import (
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// PublishWorkerConfig 表示 Kafka 异步发布专用工作池配置
type PublishWorkerConfig struct {
	WorkNum              int // WorkNum 是并发 worker 数，0 表示使用默认值
	Size                 int // Size 是任务队列容量，0 表示使用默认值
	SubmitTimeoutSeconds int // SubmitTimeoutSeconds 是入队等待超时秒数，0 表示使用 worker 默认值
}

// ConnectionConfig 表示 Kafka 连接与安全配置
type ConnectionConfig struct {
	Brokers            []string // Brokers 是 Kafka broker 地址列表
	ClientID           string   // ClientID 是客户端标识
	DialTimeoutSeconds int      // DialTimeoutSeconds 是连接超时秒数
	SecurityProtocol   string   // SecurityProtocol 是安全协议，如 PLAINTEXT、SSL、SASL_PLAINTEXT、SASL_SSL
	SASLMechanism      string   // SASLMechanism 是 SASL 机制，如 PLAIN、SCRAM-SHA-256、SCRAM-SHA-512
	Username           string   // Username 是 SASL 用户名
	Password           string   // Password 是 SASL 密码
	TLSSkipVerify      bool     // TLSSkipVerify 是否跳过 TLS 服务端证书校验
	TLSCAFile          string   // TLSCAFile 是 TLS CA 证书文件路径
}

// PublisherConfig 表示 Kafka 生产者配置
type PublisherConfig struct {
	Connection          ConnectionConfig    // Connection 是连接与安全配置
	RequiredAcks        *int                // RequiredAcks 是生产者确认级别，-1/0/1；nil 默认 -1（acks=all）
	BatchSize           int                 // BatchSize 是生产者批量发送条数，0 表示使用 kafka-go 默认值
	BatchTimeoutSeconds int                 // BatchTimeoutSeconds 是生产者批量发送超时秒数，0 表示使用 kafka-go 默认值
	PublishWorker       PublishWorkerConfig // PublishWorker 是异步发布工作池配置
}

// ConsumerManagerConfig 表示 Kafka 消费者管理器配置；单次订阅参数见 ConsumerConfig
type ConsumerManagerConfig struct {
	Connection     ConnectionConfig // Connection 是连接与安全配置
	MinBytes       int              // MinBytes 是消费者单次拉取最小字节数
	MaxBytes       int              // MaxBytes 是消费者单次拉取最大字节数
	MaxWaitSeconds int              // MaxWaitSeconds 是消费者拉取等待秒数
	StartOffset    string           // StartOffset 是消费起始偏移量，earliest 或 latest
}

// Validate 校验 Kafka 连接与安全配置
func (c ConnectionConfig) Validate() error {
	if len(c.Brokers) == 0 {
		return errors.New("brokers 不能为空")
	}
	for _, broker := range c.Brokers {
		if strings.TrimSpace(broker) == "" {
			return errors.New("brokers 不能包含空地址")
		}
	}
	if c.DialTimeoutSeconds <= 0 {
		return errors.New("dial_timeout_seconds 必须大于 0")
	}
	return validateKafkaSecurity(c)
}

// DialTimeout 返回 Kafka 连接超时
func (c ConnectionConfig) DialTimeout() time.Duration {
	return time.Duration(c.DialTimeoutSeconds) * time.Second
}

// Validate 校验 Kafka 生产者配置
func (c PublisherConfig) Validate() error {
	if err := c.Connection.Validate(); err != nil {
		return err
	}
	requiredAcks := c.RequiredAcksOrDefault()
	if requiredAcks != -1 && requiredAcks != 0 && requiredAcks != 1 {
		return errors.New("required_acks 仅支持 -1、0、1")
	}
	if c.BatchSize < 0 {
		return errors.New("batch_size 不能小于 0")
	}
	if c.BatchTimeoutSeconds < 0 {
		return errors.New("batch_timeout_seconds 不能小于 0")
	}
	return c.PublishWorker.Validate()
}

// RequiredAcksOrDefault 返回生产者确认级别，未配置默认 -1（acks=all）
func (c PublisherConfig) RequiredAcksOrDefault() int {
	if c.RequiredAcks == nil {
		return -1
	}
	return *c.RequiredAcks
}

// BatchTimeout 返回 Kafka 生产者批量发送超时，0 表示使用 kafka-go 默认值
func (c PublisherConfig) BatchTimeout() time.Duration {
	if c.BatchTimeoutSeconds <= 0 {
		return 0
	}
	return time.Duration(c.BatchTimeoutSeconds) * time.Second
}

// Validate 校验 Kafka 异步发布工作池配置
func (c PublishWorkerConfig) Validate() error {
	if c.WorkNum < 0 {
		return errors.New("publish_worker.work_num 不能小于 0")
	}
	if c.Size < 0 {
		return errors.New("publish_worker.size 不能小于 0")
	}
	if c.SubmitTimeoutSeconds < 0 {
		return errors.New("publish_worker.submit_timeout_seconds 不能小于 0")
	}
	return nil
}

// Validate 校验 Kafka 消费者管理器配置
func (c ConsumerManagerConfig) Validate() error {
	if err := c.Connection.Validate(); err != nil {
		return err
	}
	if c.MinBytes < 0 {
		return errors.New("min_bytes 不能小于 0")
	}
	if c.MaxBytes < 0 {
		return errors.New("max_bytes 不能小于 0")
	}
	if c.MaxWaitSeconds < 0 {
		return errors.New("max_wait_seconds 不能小于 0")
	}
	if startOffset := strings.TrimSpace(c.StartOffset); startOffset != "" &&
		startOffset != "earliest" && startOffset != "latest" {
		return errors.New("start_offset 仅支持 earliest 或 latest")
	}
	return nil
}

// MaxWait 返回 Kafka 消费者拉取等待时长
func (c ConsumerManagerConfig) MaxWait() time.Duration {
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
func validateKafkaSecurity(cfg ConnectionConfig) error {
	protocol := NormalizeSecurityProtocol(cfg.SecurityProtocol)
	switch protocol {
	case "PLAINTEXT", "SSL":
		return nil
	case "SASL_PLAINTEXT", "SASL_SSL":
		return validateKafkaSASLCredentials(cfg)
	default:
		return errors.Newf("security_protocol 不支持: %s", cfg.SecurityProtocol)
	}
}

// validateKafkaSASLCredentials 校验 Kafka SASL 用户名与密码
func validateKafkaSASLCredentials(cfg ConnectionConfig) error {
	if err := validateKafkaSASLMechanism(cfg.SASLMechanism); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.Username) == "" {
		return errors.New("username 不能为空")
	}
	if strings.TrimSpace(cfg.Password) == "" {
		return errors.New("password 不能为空")
	}
	return nil
}

// validateKafkaSASLMechanism 校验 Kafka SASL 机制
func validateKafkaSASLMechanism(mechanism string) error {
	switch NormalizeSASLMechanism(mechanism) {
	case "", "PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512":
		return nil
	default:
		return errors.Newf("sasl_mechanism 不支持: %s", mechanism)
	}
}
