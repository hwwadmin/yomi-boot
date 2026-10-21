// Package kafka 提供 Kafka 安全连接能力
//
// @author zwei
package kafka

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"strings"

	"gitee.com/hq_project/yomi-boot/boot/errors"

	kafkago "github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/segmentio/kafka-go/sasl/scram"
)

// securitySettings 聚合 TLS 与 SASL 安全连接参数
type securitySettings struct {
	tlsConfig   *tls.Config
	mechanism   sasl.Mechanism
	saslEnabled bool
}

// buildSecuritySettings 根据配置构建 TLS 与 SASL 安全参数
func buildSecuritySettings(cfg Config) (securitySettings, error) {
	protocol := NormalizeSecurityProtocol(cfg.SecurityProtocol)
	settings := securitySettings{}

	switch protocol {
	case "PLAINTEXT":
		return settings, nil
	case "SSL":
		tlsConfig, err := buildTLSConfig(cfg)
		if err != nil {
			return settings, err
		}
		settings.tlsConfig = tlsConfig
		return settings, nil
	case "SASL_PLAINTEXT":
		mechanism, err := buildSASLMechanism(cfg)
		if err != nil {
			return settings, err
		}
		settings.mechanism = mechanism
		settings.saslEnabled = true
		return settings, nil
	case "SASL_SSL":
		tlsConfig, err := buildTLSConfig(cfg)
		if err != nil {
			return settings, err
		}
		mechanism, err := buildSASLMechanism(cfg)
		if err != nil {
			return settings, err
		}
		settings.tlsConfig = tlsConfig
		settings.mechanism = mechanism
		settings.saslEnabled = true
		return settings, nil
	default:
		return settings, errors.Newf("kafka.security_protocol 不支持: %s", cfg.SecurityProtocol)
	}
}

// buildTLSConfig 构建 Kafka TLS 连接配置
func buildTLSConfig(cfg Config) (*tls.Config, error) {
	tlsCfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: cfg.TLSSkipVerify,
	}

	if caFile := strings.TrimSpace(cfg.TLSCAFile); caFile != "" {
		caCert, err := os.ReadFile(caFile)
		if err != nil {
			return nil, errors.Wrap(err, "读取 kafka TLS CA 证书失败")
		}

		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caCert) {
			return nil, errors.Newf("解析 kafka TLS CA 证书失败: %s", caFile)
		}
		tlsCfg.RootCAs = pool
	}

	return tlsCfg, nil
}

// buildSASLMechanism 根据配置构建 SASL 认证机制
func buildSASLMechanism(cfg Config) (sasl.Mechanism, error) {
	mechanism := NormalizeSASLMechanism(cfg.SASLMechanism)
	username := strings.TrimSpace(cfg.Username)
	password := cfg.Password

	switch mechanism {
	case "", "PLAIN":
		return plain.Mechanism{
			Username: username,
			Password: password,
		}, nil
	case "SCRAM-SHA-256":
		return scram.Mechanism(scram.SHA256, username, password)
	case "SCRAM-SHA-512":
		return scram.Mechanism(scram.SHA512, username, password)
	default:
		return nil, errors.Newf("kafka.sasl_mechanism 不支持: %s", cfg.SASLMechanism)
	}
}

// buildDialer 构建消费者使用的连接拨号器
func buildDialer(cfg Config) (*kafkago.Dialer, error) {
	security, err := buildSecuritySettings(cfg)
	if err != nil {
		return nil, err
	}

	dialer := &kafkago.Dialer{
		Timeout:   cfg.DialTimeout(),
		DualStack: true,
		ClientID:  strings.TrimSpace(cfg.ClientID),
		TLS:       security.tlsConfig,
	}
	if security.saslEnabled {
		dialer.SASLMechanism = security.mechanism
	}

	return dialer, nil
}

// buildWriterTransport 构建生产者使用的传输层，未启用 TLS/SASL 时返回 nil
func buildWriterTransport(cfg Config) (*kafkago.Transport, error) {
	security, err := buildSecuritySettings(cfg)
	if err != nil {
		return nil, err
	}
	if security.tlsConfig == nil && !security.saslEnabled {
		return nil, nil
	}

	return &kafkago.Transport{
		TLS:  security.tlsConfig,
		SASL: security.mechanism,
	}, nil
}
