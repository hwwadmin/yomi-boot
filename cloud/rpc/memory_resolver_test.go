// @author zwei

package rpc

import (
	"context"
	"testing"

	"gitee.com/hq_project/yomi-boot/core/errors"
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
