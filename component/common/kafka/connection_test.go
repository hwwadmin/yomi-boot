package kafka

import (
	"context"
	"testing"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

func TestConnection_RejectsBadDialerAndUnreachableBroker(t *testing.T) {
	if _, err := newConnection(ConnectionConfig{SecurityProtocol: "NOPE"}); err == nil {
		t.Fatal("不支持的安全协议应无法建立连接参数")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := validateConnection(ctx, connection{}, time.Second); err == nil {
		t.Fatal("未初始化拨号器应失败")
	}
	if err := validateConnection(ctx, connection{
		brokers: []string{"127.0.0.1:1"},
		dialer:  &kafkago.Dialer{Timeout: time.Second},
	}, time.Second); err == nil {
		t.Fatal("已取消的上下文拨号应失败")
	}
}
