// @author zwei

package rpc

import (
	"context"
	"testing"
	"time"

	"github.com/hwwadmin/yomi-boot/component/core/errors"
)

// 编译期验证 RPC 端点解析接口实现
var _ EndpointResolver = (*MemoryEndpointResolver)(nil)

// TestMemoryEndpointResolverResolve 验证基 URL 拼接
func TestMemoryEndpointResolverResolve(t *testing.T) {
	r := NewMemoryEndpointResolver(map[string]string{
		"user": "http://127.0.0.1:8080/",
		"api":  "https://example.test/api",
	})
	cases := []struct {
		name   string
		suffix string
		want   string
	}{
		{"user", "/list", "http://127.0.0.1:8080/list"},
		{"user", "list", "http://127.0.0.1:8080/list"},
		{"user", "", "http://127.0.0.1:8080"},
		{"api", "/v1/query", "https://example.test/api/v1/query"},
	}
	for _, tc := range cases {
		got, err := r.ResolveHTTPBase(context.Background(), tc.name, tc.suffix)
		if err != nil {
			t.Fatalf("%s %q: %v", tc.name, tc.suffix, err)
		}
		if got != tc.want {
			t.Fatalf("%s %q: got %q want %q", tc.name, tc.suffix, got, tc.want)
		}
	}
}

// TestMemoryEndpointResolverErrors 验证缺失、非法地址和取消
func TestMemoryEndpointResolverErrors(t *testing.T) {
	r := NewMemoryEndpointResolver(map[string]string{
		"bad":     "127.0.0.1:8080",
		"query":   "http://example.test/api?x=1",
		"healthy": "http://127.0.0.1:8080",
	})

	if _, err := r.ResolveHTTPBase(context.Background(), "missing", "/x"); err == nil {
		t.Fatal("未配置服务应返回错误")
	}
	if _, err := r.ResolveHTTPBase(context.Background(), "bad", "/x"); err == nil {
		t.Fatal("无协议地址应返回错误")
	}
	if _, err := r.ResolveHTTPBase(context.Background(), "query", "/x"); err == nil {
		t.Fatal("带查询串的基地址应返回错误")
	}
	if _, err := r.ResolveHTTPBase(context.Background(), "healthy", "/x"); err != nil {
		t.Fatalf("健康地址应解析成功: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.ResolveHTTPBase(ctx, "healthy", "/x"); !errors.Is(err, context.Canceled) {
		t.Fatalf("取消 ctx 应返回 context.Canceled，实际 %v", err)
	}
}

// TestMemoryEndpointResolverCopiesConfig 验证构造后修改原 map 不影响解析器
func TestMemoryEndpointResolverCopiesConfig(t *testing.T) {
	endpoints := map[string]string{"user": "http://127.0.0.1:8080"}
	r := NewMemoryEndpointResolver(endpoints)
	endpoints["user"] = "http://127.0.0.1:9090"

	got, err := r.ResolveHTTPBase(context.Background(), "user", "/x")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "http://127.0.0.1:8080/x" {
		t.Fatalf("got %q want http://127.0.0.1:8080/x", got)
	}
}

// TestMemoryEndpointResolverNilReceiver 验证 nil 接收者
func TestMemoryEndpointResolverNilReceiver(t *testing.T) {
	var r *MemoryEndpointResolver
	if _, err := r.ResolveHTTPBase(context.Background(), "user", "/x"); err == nil {
		t.Fatal("nil 接收者应返回错误")
	}
}

func TestMemoryResolverAndRetryEdges(t *testing.T) {
	resolver := NewMemoryEndpointResolver(map[string]string{" ": "http://127.0.0.1", "demo": "http://127.0.0.1:8080/"})
	if _, err := (*MemoryEndpointResolver)(nil).ResolveHTTPBase(context.Background(), "demo", "/a"); err == nil {
		t.Fatal("nil 解析器应失败")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := resolver.ResolveHTTPBase(canceled, "demo", "/a"); err == nil {
		t.Fatal("已取消的 ctx 应失败")
	}
	if _, err := resolver.ResolveHTTPBase(context.Background(), " ", "/a"); err == nil {
		t.Fatal("空服务名应失败")
	}

	if backoffBase(0, time.Second, 10*time.Second) != time.Second {
		t.Fatal("attempt 小于 1 应按 1 计算")
	}
	if backoffBase(8, time.Second, 3*time.Second) != 3*time.Second {
		t.Fatal("退避应被上限截断")
	}
	if got := jitterBackoff(0); got != 0 {
		t.Fatalf("零退避 = %v", got)
	}
	if got := jitterBackoff(time.Nanosecond); got != time.Nanosecond {
		t.Fatalf("极小退避 = %v", got)
	}
	if !canWait(context.Background(), 0) || !canWait(context.Background(), time.Second) {
		t.Fatal("无截止时间或零等待应可以等待")
	}
	short, cancelShort := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancelShort()
	if canWait(short, time.Hour) {
		t.Fatal("预算不足时不应继续等待")
	}
	if err := sleepBackoff(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if err := sleepBackoff(canceled, time.Hour); err == nil {
		t.Fatal("已取消的等待应返回错误")
	}
}
