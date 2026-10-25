package consul

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/cloud/rpc"
)

// 编译期验证 RPC 端点解析接口实现
var _ rpc.EndpointResolver = (*ConsulRPCResolver)(nil)

// TestConsulRPCResolverNilClient 验证适配器对 nil 客户端的防御
func TestConsulRPCResolverNilClient(t *testing.T) {
	if _, err := NewConsulRPCResolver(nil).ResolveHTTPBase(context.Background(), "svc", "/x"); err == nil {
		t.Fatal("nil client 应返回错误")
	}
}

// TestConsulRPCResolverDelegates 验证适配器委托发现客户端解析并传播错误
func TestConsulRPCResolverDelegates(t *testing.T) {
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Consul-Index", "1")
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/gone") {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_, _ = w.Write([]byte(`[{"Node":{"Address":"10.0.0.1"},"Service":{"Address":"","Port":8080}}]`))
	}))
	defer agent.Close()

	client, err := NewClient(Config{Addr: agent.URL, DiscoveryURLScheme: "http://"})
	if err != nil {
		t.Fatalf("创建客户端失败: %v", err)
	}
	resolver := NewConsulRPCResolver(client)

	got, err := resolver.ResolveHTTPBase(context.Background(), "svc", "/action")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if got != "http://10.0.0.1:8080/action" {
		t.Fatalf("解析结果期望 http://10.0.0.1:8080/action，实际 %s", got)
	}

	if _, err := resolver.ResolveHTTPBase(context.Background(), "gone", "/action"); err == nil {
		t.Fatal("无健康实例应传播错误")
	}
}
