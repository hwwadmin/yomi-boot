package rpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// newTestRPCClient 构造带测试服务发现的 HTTP RPC 客户端
func newTestRPCClient(serverURL string, opts ...ClientOption) *HttpRpcClient {
	return NewHttpRpcClient(resolverFunc(func(context.Context, string, string) (string, error) {
		return serverURL, nil
	}), newTestServiceName(), opts...)
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
	)
	var env testEnvelope
	err := client.Invoke(context.Background(), InvokeRequest{
		Endpoint:    "/x",
		RespBody:    &env,
		BizHook:     checkTestEnvelope,
		MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if hits.Load() != 3 {
		t.Fatalf("hits=%d want 3", hits.Load())
	}
}

// TestInvoke_AttemptTimeoutRetriesUntilAttemptsExhausted 验证单次尝试超时按 MaxAttempts 重试
func TestInvoke_AttemptTimeoutRetriesUntilAttemptsExhausted(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL, WithBackoff(time.Millisecond, time.Millisecond))
	var env testEnvelope
	err := client.Invoke(context.Background(), InvokeRequest{
		Endpoint:    "/x",
		RespBody:    &env,
		BizHook:     checkTestEnvelope,
		Timeout:     20 * time.Millisecond,
		MaxAttempts: 3,
	})
	if err == nil {
		t.Fatal("全部尝试超时应返回错误")
	}
	if hits.Load() != 3 {
		t.Fatalf("单次超时应重试到 MaxAttempts，hits=%d", hits.Load())
	}
}

// TestInvoke_AttemptTimeoutRetriesThenSucceeds 验证单次超时后重试成功
func TestInvoke_AttemptTimeoutRetriesThenSucceeds(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			time.Sleep(100 * time.Millisecond)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"OK"}`))
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL, WithBackoff(time.Millisecond, time.Millisecond))
	var env testEnvelope
	err := client.Invoke(context.Background(), InvokeRequest{
		Endpoint:    "/x",
		RespBody:    &env,
		BizHook:     checkTestEnvelope,
		Timeout:     20 * time.Millisecond,
		MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("单次超时应重试并成功: %v", err)
	}
	if hits.Load() != 2 {
		t.Fatalf("hits=%d want 2", hits.Load())
	}
}

// TestInvoke_CallerDeadlineStopsAttemptRetry 验证调用方 deadline 到期后不再重试
func TestInvoke_CallerDeadlineStopsAttemptRetry(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL, WithBackoff(time.Millisecond, time.Millisecond))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()
	var env testEnvelope
	err := client.Invoke(ctx, InvokeRequest{
		Endpoint:    "/x",
		RespBody:    &env,
		BizHook:     checkTestEnvelope,
		Timeout:     50 * time.Millisecond,
		MaxAttempts: 3,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("期望调用方 DeadlineExceeded，实际 %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("调用方到期后不应重试，hits=%d", hits.Load())
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

// TestInvoke_RetryAfterHonored 验证内部重试等待 Retry-After
func TestInvoke_RetryAfterHonored(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"OK"}`))
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL, WithBackoff(time.Millisecond, time.Millisecond))
	var env testEnvelope
	start := time.Now()
	err := client.Invoke(context.Background(), InvokeRequest{
		Endpoint:    "/x",
		RespBody:    &env,
		BizHook:     checkTestEnvelope,
		MaxAttempts: 2,
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("等待 Retry-After 后应成功: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
		t.Fatalf("应等待约 1s，实际 %v", elapsed)
	}
	if hits.Load() != 2 {
		t.Fatalf("hits=%d want 2", hits.Load())
	}
}

// TestInvoke_RetryAfterExceedsBudget 验证 Retry-After 放不进调用方预算时直接返回下游错误
func TestInvoke_RetryAfterExceedsBudget(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL, WithBackoff(time.Millisecond, time.Millisecond))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var env testEnvelope
	err := client.Invoke(ctx, InvokeRequest{
		Endpoint:    "/x",
		RespBody:    &env,
		BizHook:     checkTestEnvelope,
		MaxAttempts: 3,
		Timeout:     3 * time.Second,
	})
	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) || statusErr.Code != http.StatusServiceUnavailable {
		t.Fatalf("期望 HTTP 503，实际 %v", err)
	}
	if statusErr.RetryAfter != 2*time.Second {
		t.Fatalf("RetryAfter: got %v want 2s", statusErr.RetryAfter)
	}
	if hits.Load() != 1 {
		t.Fatalf("调用方预算不足时不应重试，hits=%d", hits.Load())
	}
}

// TestInvoke_DiscoveryBudgetExhaustedPreservesError 验证发现重试放不进调用方预算时返回发现错误
func TestInvoke_DiscoveryBudgetExhaustedPreservesError(t *testing.T) {
	var calls atomic.Int64
	client := NewHttpRpcClient(resolverFunc(func(context.Context, string, string) (string, error) {
		calls.Add(1)
		return "", errors.New("consul down")
	}), "svc",
		WithBackoff(200*time.Millisecond, 200*time.Millisecond),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	var env testEnvelope
	err := client.Invoke(ctx, InvokeRequest{
		Endpoint:    "/x",
		RespBody:    &env,
		BizHook:     checkTestEnvelope,
		MaxAttempts: 3,
		Timeout:     3 * time.Second,
	})
	var discoveryErr *discoveryError
	if !errors.As(err, &discoveryErr) {
		t.Fatalf("期望发现错误，实际 %v", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("不应把发现错误替换成 DeadlineExceeded: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("调用方预算不足时不应继续重试，resolver 调用 %d 次", calls.Load())
	}
}
