package consultest_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hwwadmin/yomi-boot/component/cloud/consul/consultest"
)

// TestRegisterResolveDeregister 验证无健康检查的实例可被发现，注销后发现失败。
func TestRegisterResolveDeregister(t *testing.T) {
	client, prefix, cleanup := consultest.NewClient(t)
	defer cleanup()

	serviceName, serviceID := consultest.Register(t, prefix, "orders", "10.0.0.8", 8080)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	got, err := client.ResolveHTTPBase(ctx, serviceName, "/action")
	if err != nil {
		t.Fatalf("发现实例失败: %v", err)
	}
	const want = "10.0.0.8:8080/action"
	if got != want {
		t.Fatalf("发现地址 = %q, want %q", got, want)
	}

	consultest.Deregister(t, serviceID)

	deadline := time.Now().Add(5 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		_, last = client.ResolveHTTPBase(ctx, serviceName, "/action")
		if last != nil {
			if !strings.Contains(last.Error(), "未发现健康实例") && !strings.Contains(last.Error(), "无健康实例") {
				t.Fatalf("注销后的错误 = %v, 应表示没有健康实例", last)
			}
			return
		}
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			t.Fatalf("等待注销生效超时: %v", ctx.Err())
		case <-timer.C:
		}
	}
	t.Fatal("注销后仍能发现实例")
}
