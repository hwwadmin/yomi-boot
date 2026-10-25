// Kafka 安全连接构建的单元测试
//
// @author zwei
package kafka

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/segmentio/kafka-go/sasl/plain"
)

// writeTestCAPEM 生成自签名 CA 证书 PEM 文件，返回文件路径
func writeTestCAPEM(t *testing.T) string {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("生成私钥失败: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "yomi-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("生成证书失败: %v", err)
	}
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("写入证书文件失败: %v", err)
	}
	return path
}

// TestBuildSecuritySettings_Plaintext 验证明文协议返回空安全配置
func TestBuildSecuritySettings_Plaintext(t *testing.T) {
	settings, err := buildSecuritySettings(ConnectionConfig{SecurityProtocol: "plaintext"})
	if err != nil {
		t.Fatalf("PLAINTEXT 构建失败: %v", err)
	}
	if settings.tlsConfig != nil || settings.mechanism != nil || settings.saslEnabled {
		t.Fatalf("PLAINTEXT 不应启用安全配置: %+v", settings)
	}
}

// TestBuildSecuritySettings_TLS 验证 SSL 协议构建 TLS 配置与 CA 文件解析
func TestBuildSecuritySettings_TLS(t *testing.T) {
	caFile := writeTestCAPEM(t)
	settings, err := buildSecuritySettings(ConnectionConfig{
		SecurityProtocol: "SSL",
		TLSCAFile:        caFile,
		TLSSkipVerify:    true,
	})
	if err != nil {
		t.Fatalf("SSL 构建失败: %v", err)
	}
	if settings.tlsConfig == nil {
		t.Fatal("SSL 应构建 TLS 配置")
	}
	if settings.tlsConfig.RootCAs == nil {
		t.Fatal("配置了 CA 文件应加载证书池")
	}
	if !settings.tlsConfig.InsecureSkipVerify {
		t.Fatal("TLSSkipVerify 应透传")
	}
	if settings.saslEnabled || settings.mechanism != nil {
		t.Fatalf("SSL 不应启用 SASL: %+v", settings)
	}
}

// TestBuildSecuritySettings_TLSInvalidCAFile 验证 CA 文件缺失或内容非法时报错
func TestBuildSecuritySettings_TLSInvalidCAFile(t *testing.T) {
	badFile := filepath.Join(t.TempDir(), "bad.pem")
	if err := os.WriteFile(badFile, []byte("not a pem"), 0o600); err != nil {
		t.Fatalf("准备非法证书文件失败: %v", err)
	}

	if _, err := buildSecuritySettings(ConnectionConfig{SecurityProtocol: "SSL", TLSCAFile: "missing/ca.pem"}); err == nil {
		t.Fatal("CA 文件缺失应返回错误")
	}
	if _, err := buildSecuritySettings(ConnectionConfig{SecurityProtocol: "SSL", TLSCAFile: badFile}); err == nil {
		t.Fatal("CA 文件内容非法应返回错误")
	}
}

// TestBuildSecuritySettings_SASL 验证 SASL 各机制与凭据缺失报错
func TestBuildSecuritySettings_SASL(t *testing.T) {
	creds := func(mechanism string) ConnectionConfig {
		return ConnectionConfig{
			SecurityProtocol: "SASL_PLAINTEXT",
			SASLMechanism:    mechanism,
			Username:         "user",
			Password:         "pass",
		}
	}

	plainSettings, err := buildSecuritySettings(creds("PLAIN"))
	if err != nil {
		t.Fatalf("PLAIN 构建失败: %v", err)
	}
	if !plainSettings.saslEnabled {
		t.Fatal("SASL_PLAINTEXT 应启用 SASL")
	}
	if _, ok := plainSettings.mechanism.(plain.Mechanism); !ok {
		t.Fatalf("PLAIN 机制类型不符: %T", plainSettings.mechanism)
	}

	for _, mechanism := range []string{"SCRAM-SHA-256", "SCRAM-SHA-512"} {
		settings, err := buildSecuritySettings(creds(mechanism))
		if err != nil {
			t.Fatalf("%s 构建失败: %v", mechanism, err)
		}
		if got := settings.mechanism.Name(); got != mechanism {
			t.Fatalf("%s 机制名不符: %s", mechanism, got)
		}
	}

	if _, err := buildSecuritySettings(creds("GSSAPI")); err == nil {
		t.Fatal("不支持的 SASL 机制应返回错误")
	}

	// 凭据校验由配置层（validateKafkaSASLCredentials）负责，构建层允许空凭据
	empty := ConnectionConfig{SecurityProtocol: "SASL_PLAINTEXT", SASLMechanism: "PLAIN"}
	if _, err := buildSecuritySettings(empty); err != nil {
		t.Fatalf("构建层不应校验凭据: %v", err)
	}
}

// TestBuildSecuritySettings_SASLSSL 验证 SASL_SSL 同时构建 TLS 与 SASL
func TestBuildSecuritySettings_SASLSSL(t *testing.T) {
	settings, err := buildSecuritySettings(ConnectionConfig{
		SecurityProtocol: "SASL_SSL",
		SASLMechanism:    "PLAIN",
		Username:         "user",
		Password:         "pass",
	})
	if err != nil {
		t.Fatalf("SASL_SSL 构建失败: %v", err)
	}
	if settings.tlsConfig == nil || !settings.saslEnabled || settings.mechanism == nil {
		t.Fatalf("SASL_SSL 应同时启用 TLS 与 SASL: %+v", settings)
	}
}

// TestBuildSecuritySettings_UnsupportedProtocol 验证未知协议报错
func TestBuildSecuritySettings_UnsupportedProtocol(t *testing.T) {
	if _, err := buildSecuritySettings(ConnectionConfig{SecurityProtocol: "HTTP"}); err == nil {
		t.Fatal("未知协议应返回错误")
	}
}

// TestBuildWriterTransport 验证明文不构建传输层、安全协议构建传输层
func TestBuildWriterTransport(t *testing.T) {
	transport, err := buildWriterTransport(ConnectionConfig{SecurityProtocol: "PLAINTEXT"})
	if err != nil {
		t.Fatalf("PLAINTEXT 构建传输层失败: %v", err)
	}
	if transport != nil {
		t.Fatalf("PLAINTEXT 不应构建传输层: %+v", transport)
	}

	transport, err = buildWriterTransport(ConnectionConfig{
		SecurityProtocol: "SASL_PLAINTEXT",
		SASLMechanism:    "PLAIN",
		Username:         "user",
		Password:         "pass",
	})
	if err != nil {
		t.Fatalf("SASL_PLAINTEXT 构建传输层失败: %v", err)
	}
	if transport == nil || transport.SASL == nil {
		t.Fatalf("SASL_PLAINTEXT 应构建带 SASL 的传输层: %+v", transport)
	}
}

// TestBuildDialer 验证消费者拨号器透传超时、ClientID 与 SASL
func TestBuildDialer(t *testing.T) {
	dialer, err := buildDialer(ConnectionConfig{
		Brokers:            []string{"127.0.0.1:9092"},
		ClientID:           "yomi-test",
		DialTimeoutSeconds: 3,
		SecurityProtocol:   "SASL_PLAINTEXT",
		SASLMechanism:      "SCRAM-SHA-256",
		Username:           "user",
		Password:           "pass",
	})
	if err != nil {
		t.Fatalf("构建拨号器失败: %v", err)
	}
	if dialer.ClientID != "yomi-test" {
		t.Fatalf("ClientID 透传不符: %s", dialer.ClientID)
	}
	if dialer.Timeout != 3*time.Second {
		t.Fatalf("超时透传不符: %v", dialer.Timeout)
	}
	if dialer.SASLMechanism == nil {
		t.Fatal("SASL 机制应挂载到拨号器")
	}

	dialer, err = buildDialer(ConnectionConfig{ClientID: "yomi-test"})
	if err != nil {
		t.Fatalf("构建明文拨号器失败: %v", err)
	}
	if dialer.SASLMechanism != nil || dialer.TLS != nil {
		t.Fatalf("明文拨号器不应带安全配置: %+v", dialer)
	}
}
