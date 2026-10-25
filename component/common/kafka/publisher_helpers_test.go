// Kafka 发布辅助函数与配置默认值的单元测试
//
// @author zwei
package kafka

import (
	"testing"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

// TestEffectiveBatchSize 验证批量条数默认值
func TestEffectiveBatchSize(t *testing.T) {
	cases := []struct {
		in   int
		want int
	}{
		{in: 0, want: 100},
		{in: -5, want: 100},
		{in: 50, want: 50},
	}
	for _, tc := range cases {
		if got := effectiveBatchSize(tc.in); got != tc.want {
			t.Fatalf("effectiveBatchSize(%d) 期望 %d，实际 %d", tc.in, tc.want, got)
		}
	}
}

// TestEffectiveBatchTimeoutSeconds 验证批量超时默认值
func TestEffectiveBatchTimeoutSeconds(t *testing.T) {
	cases := []struct {
		in   int
		want int
	}{
		{in: 0, want: 1},
		{in: -1, want: 1},
		{in: 5, want: 5},
	}
	for _, tc := range cases {
		if got := effectiveBatchTimeoutSeconds(tc.in); got != tc.want {
			t.Fatalf("effectiveBatchTimeoutSeconds(%d) 期望 %d，实际 %d", tc.in, tc.want, got)
		}
	}
}

// TestBuildPublishWorkerConfig 验证工作池配置默认值与透传
func TestBuildPublishWorkerConfig(t *testing.T) {
	def := buildPublishWorkerConfig(PublishWorkerConfig{})
	if def.WorkNum != 4 || def.Size != 256 {
		t.Fatalf("默认工作池配置期望 4/256，实际 %d/%d", def.WorkNum, def.Size)
	}
	if def.SubmitTimeout != 0 {
		t.Fatalf("未配置 submit_timeout 不应设置超时，实际 %v", def.SubmitTimeout)
	}

	custom := buildPublishWorkerConfig(PublishWorkerConfig{
		WorkNum:              8,
		Size:                 512,
		SubmitTimeoutSeconds: 7,
	})
	if custom.WorkNum != 8 || custom.Size != 512 {
		t.Fatalf("自定义工作池配置不符: %d/%d", custom.WorkNum, custom.Size)
	}
	if custom.SubmitTimeout != 7*time.Second {
		t.Fatalf("submit_timeout 透传不符: %v", custom.SubmitTimeout)
	}
}

// TestToKafkaHeaders 验证消息头转换与空值短路
func TestToKafkaHeaders(t *testing.T) {
	if got := toKafkaHeaders(nil); got != nil {
		t.Fatalf("空消息头应返回 nil，实际 %v", got)
	}
	got := toKafkaHeaders([]Header{{Key: "trace", Value: []byte("abc")}})
	if len(got) != 1 || got[0].Key != "trace" || string(got[0].Value) != "abc" {
		t.Fatalf("消息头转换不符: %+v", got)
	}
}

// TestToKafkaMessage 验证发布请求到 kafka-go 消息的映射
func TestToKafkaMessage(t *testing.T) {
	req := PublishRequest{
		Topic: "orders",
		Key:   []byte("k1"),
		Value: []byte("v1"),
		Headers: []Header{
			{Key: "h1", Value: []byte("hv1")},
		},
	}
	msg := toKafkaMessage(req)
	if msg.Topic != "orders" || string(msg.Key) != "k1" || string(msg.Value) != "v1" {
		t.Fatalf("消息映射不符: %+v", msg)
	}
	if len(msg.Headers) != 1 || msg.Headers[0].Key != "h1" {
		t.Fatalf("消息头映射不符: %+v", msg.Headers)
	}

	empty := toKafkaMessage(PublishRequest{Topic: "t", Value: []byte("v")})
	if empty.Headers != nil {
		t.Fatalf("无消息头时 Headers 应为 nil，实际 %v", empty.Headers)
	}
}

// TestValidatePublishRequest 验证发布请求参数校验
func TestValidatePublishRequest(t *testing.T) {
	if err := validatePublishRequest(PublishRequest{Topic: "", Value: []byte("v")}); err == nil {
		t.Fatal("空 topic 应返回错误")
	}
	if err := validatePublishRequest(PublishRequest{Topic: "t", Value: nil}); err == nil {
		t.Fatal("空消息体应返回错误")
	}
	if err := validatePublishRequest(PublishRequest{Topic: "t", Value: []byte("v")}); err != nil {
		t.Fatalf("合法请求不应报错: %v", err)
	}
}

// TestBuildWriter 验证 Writer 的确认级别与批量参数
func TestBuildWriter(t *testing.T) {
	requiredAcks := 0
	writer, err := buildWriter(PublisherConfig{
		Connection:          ConnectionConfig{SecurityProtocol: "PLAINTEXT"},
		RequiredAcks:        &requiredAcks,
		BatchSize:           20,
		BatchTimeoutSeconds: 2,
	}, []string{"127.0.0.1:9092"})
	if err != nil {
		t.Fatalf("构建 Writer 失败: %v", err)
	}
	if writer.RequiredAcks != kafkago.RequireNone {
		t.Fatalf("acks=0 应映射 RequireNone，实际 %v", writer.RequiredAcks)
	}
	if writer.BatchSize != 20 {
		t.Fatalf("BatchSize 透传不符: %d", writer.BatchSize)
	}
	if writer.BatchTimeout != 2*time.Second {
		t.Fatalf("BatchTimeout 透传不符: %v", writer.BatchTimeout)
	}
	if writer.Transport != nil {
		t.Fatalf("明文不应构建 Transport: %+v", writer.Transport)
	}

	// 未配置批量参数时不覆盖 kafka-go 默认值
	writer, err = buildWriter(PublisherConfig{
		Connection: ConnectionConfig{SecurityProtocol: "PLAINTEXT"},
	}, []string{"127.0.0.1:9092"})
	if err != nil {
		t.Fatalf("构建 Writer 失败: %v", err)
	}
	if writer.BatchSize != 0 || writer.BatchTimeout != 0 {
		t.Fatalf("未配置批量参数不应覆盖默认值: %d/%v", writer.BatchSize, writer.BatchTimeout)
	}
}

// TestConsumerManagerConfigMaxWait 验证拉取等待时长换算
func TestConsumerManagerConfigMaxWait(t *testing.T) {
	if got := (ConsumerManagerConfig{MaxWaitSeconds: 2}).MaxWait(); got != 2*time.Second {
		t.Fatalf("MaxWait 期望 2s，实际 %v", got)
	}
	if got := (ConsumerManagerConfig{}).MaxWait(); got != 0 {
		t.Fatalf("未配置 MaxWait 期望 0，实际 %v", got)
	}
}

// TestPublisherConfigBatchTimeout 验证批量发送超时换算
func TestPublisherConfigBatchTimeout(t *testing.T) {
	if got := (PublisherConfig{BatchTimeoutSeconds: 0}).BatchTimeout(); got != 0 {
		t.Fatalf("未配置 BatchTimeout 期望 0，实际 %v", got)
	}
	if got := (PublisherConfig{BatchTimeoutSeconds: 3}).BatchTimeout(); got != 3*time.Second {
		t.Fatalf("BatchTimeout 期望 3s，实际 %v", got)
	}
}

// TestValidateKafkaSASLMechanism 验证 SASL 机制合法域
func TestValidateKafkaSASLMechanism(t *testing.T) {
	for _, mechanism := range []string{"", "plain", "SCRAM_SHA_256", "SCRAM-SHA-512"} {
		if err := validateKafkaSASLMechanism(mechanism); err != nil {
			t.Fatalf("机制 %q 应合法，实际 %v", mechanism, err)
		}
	}
	if err := validateKafkaSASLMechanism("GSSAPI"); err == nil {
		t.Fatal("不支持的机制应返回错误")
	}
}

// TestValidateKafkaSASLCredentials 验证 SASL 凭据校验
func TestValidateKafkaSASLCredentials(t *testing.T) {
	valid := ConnectionConfig{SASLMechanism: "PLAIN", Username: "u", Password: "p"}
	if err := validateKafkaSASLCredentials(valid); err != nil {
		t.Fatalf("合法凭据不应报错: %v", err)
	}

	noUser := valid
	noUser.Username = " "
	if err := validateKafkaSASLCredentials(noUser); err == nil {
		t.Fatal("用户名为空应返回错误")
	}
	noPass := valid
	noPass.Password = ""
	if err := validateKafkaSASLCredentials(noPass); err == nil {
		t.Fatal("密码为空应返回错误")
	}
	badMechanism := valid
	badMechanism.SASLMechanism = "GSSAPI"
	if err := validateKafkaSASLCredentials(badMechanism); err == nil {
		t.Fatal("机制非法应返回错误")
	}
}
