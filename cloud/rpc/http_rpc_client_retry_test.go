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
	cfg := applyHTTPClientOptions(opts)
	c := &HttpRpcClient{
		serviceName: newTestServiceName(),
		resolver: resolverFunc(func(context.Context, string, string) (string, error) {
			return serverURL, nil
		}),
		httpClient: &http.Client{Timeout: cfg.HTTPTimeout},
		opts:       cfg,
		breaker:    newCircuitBreaker(cfg.Breaker),
	}
	return c
}

// TestInvoke_RetriesThenSucceeds 验证传输失败重试后成功
func TestInvoke_RetriesThenSucceeds(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"OK","data":{"ok":true}}`))
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL,
		WithBackoff(1*time.Millisecond, 2*time.Millisecond),
		WithHTTPTimeout(2*time.Second),
	)
	var env testEnvelope
	if err := invokeTest(client, context.Background(), "/x", nil, &env, checkTestEnvelope); err != nil {
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
		_, _ = w.Write([]byte(`{"code":"INVALID_INPUT","msg":"bad"}`))
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL, WithBackoff(time.Millisecond, time.Millisecond))
	var env testEnvelope
	err := invokeTest(client, context.Background(), "/x", nil, &env, checkTestEnvelope)
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

// TestInvoke_500CountsAsFailureButNotRetried 验证 500 计熔断失败但不重试
func TestInvoke_500CountsAsFailureButNotRetried(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL,
		WithBreaker(BreakerOptions{
			WindowSize: 1, MinRequests: 1, FailureRate: 1, SlowRate: 1,
			SlowThreshold: time.Minute, OpenDuration: time.Minute,
		}),
	)
	var env testEnvelope
	err := invokeTest(client, context.Background(), "/x", nil, &env, checkTestEnvelope)
	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) || statusErr.Code != http.StatusInternalServerError {
		t.Fatalf("期望 HTTP 500，实际 %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("500 不应重试，hits=%d", hits.Load())
	}
	if state, _ := breakerSnapshot(client); state != breakerOpen {
		t.Fatalf("500 应计失败并打开熔断，state=%d", state)
	}
	if err := invokeTest(client, context.Background(), "/x", nil, &env, checkTestEnvelope); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("期望 ErrCircuitOpen，实际 %v", err)
	}
}

// TestInvoke_4xxDoesNotTripBreaker 验证 4xx 不计熔断失败
func TestInvoke_4xxDoesNotTripBreaker(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL,
		WithBreaker(BreakerOptions{
			WindowSize: 1, MinRequests: 1, FailureRate: 1, SlowRate: 1,
			SlowThreshold: time.Minute, OpenDuration: time.Minute,
		}),
	)
	var env testEnvelope
	for i := 0; i < 2; i++ {
		err := invokeTest(client, context.Background(), "/x", nil, &env, checkTestEnvelope)
		var statusErr *HTTPStatusError
		if !errors.As(err, &statusErr) || statusErr.Code != http.StatusBadRequest {
			t.Fatalf("第 %d 次期望 HTTP 400，实际 %v", i+1, err)
		}
	}
	if hits.Load() != 2 {
		t.Fatalf("4xx 不应打开熔断，hits=%d", hits.Load())
	}
}

// TestInvoke_DiscoveryErrorNotCountedAsBreaker 验证发现失败可重试但不计下游熔断
func TestInvoke_DiscoveryErrorNotCountedAsBreaker(t *testing.T) {
	var calls atomic.Int64
	client := newTestRPCClient("",
		WithBreaker(BreakerOptions{
			WindowSize: 1, MinRequests: 1, FailureRate: 1, SlowRate: 1,
			SlowThreshold: time.Minute, OpenDuration: time.Minute,
		}),
	)
	client.resolver = resolverFunc(func(context.Context, string, string) (string, error) {
		calls.Add(1)
		return "", errors.New("consul down")
	})

	err := invokeTestWith(client, context.Background(), "/x", nil, nil, NoopBizHook, 1)
	var discoveryErr *discoveryError
	if !errors.As(err, &discoveryErr) {
		t.Fatalf("期望 discoveryError，实际 %v", err)
	}
	if state, _ := breakerSnapshot(client); state != breakerClosed {
		t.Fatalf("发现失败不应打开下游熔断，state=%d", state)
	}

	_ = invokeTestWith(client, context.Background(), "/x", nil, nil, NoopBizHook, 1)
	if calls.Load() != 2 {
		t.Fatalf("发现失败不应打开熔断，calls=%d", calls.Load())
	}
}

// TestInvoke_LocalRequestErrorNotRetried 验证本地请求构建错误不重试、不计熔断
func TestInvoke_LocalRequestErrorNotRetried(t *testing.T) {
	var calls atomic.Int64
	client := newTestRPCClient("",
		WithBreaker(BreakerOptions{
			WindowSize: 1, MinRequests: 1, FailureRate: 1, SlowRate: 1,
			SlowThreshold: time.Minute, OpenDuration: time.Minute,
		}),
	)
	client.resolver = resolverFunc(func(context.Context, string, string) (string, error) {
		calls.Add(1)
		return "10.0.0.1:8080/x", nil
	})

	err := invokeTest(client, context.Background(), "/x", nil, nil, NoopBizHook)
	var localErr *requestError
	if !errors.As(err, &localErr) {
		t.Fatalf("期望 requestError，实际 %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("本地错误不应重试，calls=%d", calls.Load())
	}
	if state, _ := breakerSnapshot(client); state != breakerClosed {
		t.Fatalf("本地错误不应打开熔断，state=%d", state)
	}
}

// TestInvoke_RequestMaxAttemptsOverride 验证按调用覆盖最大尝试次数
func TestInvoke_RequestMaxAttemptsOverride(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL,
		WithBackoff(time.Millisecond, time.Millisecond),
	)
	var env testEnvelope
	err := client.Invoke(context.Background(), InvokeRequest{
		Endpoint:    "/x",
		RespBody:    &env,
		BizHook:     checkTestEnvelope,
		MaxAttempts: 1,
	})
	if err == nil {
		t.Fatal("502 应返回错误")
	}
	if hits.Load() != 1 {
		t.Fatalf("请求级 MaxAttempts=1 应只尝试一次，hits=%d", hits.Load())
	}
}
