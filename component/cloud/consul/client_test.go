// Package consul 的客户端 HTTP Timeout 行为单测
//
// @author zwei
package consul

import (
	"context"
	"testing"
	"time"
)

// TestNewClientAppliesHTTPTimeout 验证 NewClient 将配置超时写入 Client.HTTPTimeout
func TestNewClientAppliesHTTPTimeout(t *testing.T) {
	t.Parallel()

	client, err := NewClient(Config{
		Addr:               "127.0.0.1:8500",
		HTTPTimeoutSeconds: 2,
	})
	if err != nil {
		t.Fatalf("创建客户端失败: %v", err)
	}
	if got := client.HTTPTimeout(); got != 2*time.Second {
		t.Fatalf("HTTPTimeout 期望 2s，实际 %s", got)
	}
}

// TestCacheTTLConfigured 验证发现缓存 TTL 的配置与默认值
func TestCacheTTLConfigured(t *testing.T) {
	t.Parallel()

	client, err := NewClient(Config{Addr: "127.0.0.1:8500", DiscoveryCacheSeconds: 5})
	if err != nil {
		t.Fatalf("创建客户端失败: %v", err)
	}
	if got := client.CacheTTL(); got != 5*time.Second {
		t.Fatalf("CacheTTL 期望 5s，实际 %s", got)
	}

	fallback, err := NewClient(Config{Addr: "127.0.0.1:8500"})
	if err != nil {
		t.Fatalf("创建客户端失败: %v", err)
	}
	if got := fallback.CacheTTL(); got != 30*time.Second {
		t.Fatalf("未配置时 CacheTTL 期望默认 30s，实际 %s", got)
	}
}

// TestMustNewClient 验证 Fail Fast 构造的成功路径
func TestMustNewClient(t *testing.T) {
	t.Parallel()

	client := MustNewClient(Config{Addr: "127.0.0.1:8500"})
	if client == nil {
		t.Fatal("MustNewClient 不应返回 nil")
	}
	if client.l1Cache == nil {
		t.Fatal("MustNewClient 应初始化一级缓存")
	}
}

// TestResolveHTTPBaseHonorsHTTPTimeout 验证 Agent 不可达时，发现请求在 HttpClient.Timeout 内失败返回
func TestResolveHTTPBaseHonorsHTTPTimeout(t *testing.T) {
	t.Parallel()

	// 使用测试网黑洞地址，TCP 握手会挂起直至客户端超时
	client, err := NewClient(Config{
		Addr:               "10.255.255.1:8500",
		HTTPTimeoutSeconds: 1,
	})
	if err != nil {
		t.Fatalf("创建客户端失败: %v", err)
	}

	start := time.Now()
	_, err = client.ResolveHTTPBase(context.Background(), "any-service", "/ping")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("不可达 Consul 应返回错误")
	}
	// 允许少量调度抖动：下限略小于 1s，上限远小于无 Timeout 时的长时间阻塞
	if elapsed < 800*time.Millisecond || elapsed > 3*time.Second {
		t.Fatalf("应在约 1s HTTP Timeout 内失败，实际耗时 %s, err=%v", elapsed, err)
	}
}
