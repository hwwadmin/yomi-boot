package dtm

import (
	"context"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"

	"github.com/dtm-labs/client/dtmcli"
)

// TestNewClientDisabled 验证 server 为空时客户端为未启用状态
func TestNewClientDisabled(t *testing.T) {
	client := NewClient(nil, "")
	if client == nil {
		t.Fatal("NewClient 不应返回 nil")
	}
	if client.Enabled() {
		t.Fatal("server 为空时应为未启用")
	}

	saga, err := client.NewSaga(context.Background())
	if saga != nil || !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("未启用时应返回 ErrNotConfigured，实际 saga=%v err=%v", saga, err)
	}
}

// TestNewClientRejectsInvalidServer 验证启动装配时拒绝不可用的 server 地址
func TestNewClientRejectsInvalidServer(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("非法 server 地址应在构造期 panic")
		}
	}()
	_ = NewClient(nil, "ftp://127.0.0.1:36789")
}

// TestNewClientTimeout 验证超时选项与默认值
func TestNewClientTimeout(t *testing.T) {
	tests := []struct {
		name   string
		option Option
		want   time.Duration
	}{
		{name: "默认", option: nil, want: DefaultTimeout},
		{name: "显式", option: WithTimeout(5 * time.Second), want: 5 * time.Second},
		{name: "零值回落默认", option: WithTimeout(0), want: DefaultTimeout},
		{name: "负数回落默认", option: WithTimeout(-time.Second), want: DefaultTimeout},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var client *Client
			if tc.option == nil {
				client = NewClient(nil, "")
			} else {
				client = NewClient(nil, "", tc.option)
			}
			if got := client.effectiveTimeout(); got != tc.want {
				t.Fatalf("超时期望 %s，实际 %s", tc.want, got)
			}
		})
	}
}

// TestClientNilGuards 验证 nil 客户端的防御
func TestClientNilGuards(t *testing.T) {
	var client *Client
	if client.Enabled() {
		t.Fatal("nil Client 应为未启用")
	}
	if got := client.effectiveTimeout(); got != DefaultTimeout {
		t.Fatalf("nil Client 超时应回退默认值，实际 %s", got)
	}
}

// TestClientZeroValueDisabled 验证零值 Client 不会被误用为已启用
func TestClientZeroValueDisabled(t *testing.T) {
	var client Client
	if client.Enabled() {
		t.Fatal("零值 Client 应为未启用")
	}
	if _, err := client.NewSaga(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("零值 Client 应返回 ErrNotConfigured，实际: %v", err)
	}
	saga := dtmcli.NewSaga("http://127.0.0.1:36789", "g-1")
	if err := client.AddSagaStep(context.Background(), saga, "svc", "/a", "/c", nil); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("零值 Client 的 AddSagaStep 应返回 ErrNotConfigured，实际: %v", err)
	}
}
