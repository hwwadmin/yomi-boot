package rpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/boot/errors"
)

// newTestRPCClient 构造带测试服务发现的 HTTP RPC 客户端
func newTestRPCClient(serverURL string, opts ...ClientOption) *HttpRpcClient {
	cfg := applyClientOptions(opts)
	c := &HttpRpcClient{
		serviceName: "test-svc",
		httpClient:  &http.Client{Timeout: cfg.HTTPTimeout},
		opts:        cfg,
		breaker:     newCircuitBreaker(cfg.BreakerFailures, cfg.BreakerOpenDuration),
	}
	c.testResolve = func(context.Context, string) (string, error) {
		return serverURL, nil
	}
	return c
}

// TestInvoke_RetriesThenSucceeds 验证传输失败重试后成功
func TestInvoke_RetriesThenSucceeds(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := hits.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"OK","msg":"","timestamp":1,"data":{"ok":true}}`))
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL,
		WithMaxAttempts(3),
		WithBackoff(1*time.Millisecond, 2*time.Millisecond),
		WithHTTPTimeout(2*time.Second),
	)
	var resp map[string]any
	if err := client.Invoke(context.Background(), "/x", nil, &resp); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if hits.Load() != 3 {
		t.Fatalf("hits=%d want 3", hits.Load())
	}
}

// TestInvoke_BizErrorNotRetried 验证业务错误不重试
func TestInvoke_BizErrorNotRetried(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"INVALID_INPUT","msg":"bad","timestamp":1}`))
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL, WithMaxAttempts(3), WithBackoff(time.Millisecond, time.Millisecond))
	err := client.Invoke(context.Background(), "/x", nil, nil)
	if err == nil {
		t.Fatal("期望业务错误")
	}
	var biz *errors.BizError
	if !errors.As(err, &biz) || biz.Code != "INVALID_INPUT" {
		t.Fatalf("期望 INVALID_INPUT，实际 %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("业务错误不应重试，hits=%d", hits.Load())
	}
}

// TestInvoke_CircuitBreakerOpens 验证连续失败后熔断打开
func TestInvoke_CircuitBreakerOpens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL,
		WithMaxAttempts(1),
		WithBreaker(2, time.Minute),
		WithHTTPTimeout(2*time.Second),
	)

	_ = client.Invoke(context.Background(), "/x", nil, nil)
	_ = client.Invoke(context.Background(), "/x", nil, nil)
	err := client.Invoke(context.Background(), "/x", nil, nil)
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("期望熔断打开，实际 %v", err)
	}
}
