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

// TestInvoke_ClientTimeoutCountsAsFailure 验证客户端 HTTP 超时计入熔断失败
func TestInvoke_ClientTimeoutCountsAsFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL,
		WithMaxAttempts(1),
		WithHTTPTimeout(20*time.Millisecond),
		WithBreaker(BreakerOptions{
			WindowSize: 1, MinRequests: 1, FailureRate: 1, SlowRate: 1,
			SlowThreshold: time.Minute, OpenDuration: time.Minute,
		}),
	)
	var env testEnvelope
	if err := client.Invoke(context.Background(), "/x", nil, &env, checkTestEnvelope); err == nil {
		t.Fatal("期望客户端超时")
	}
	if state, _ := breakerSnapshot(client); state != breakerOpen {
		t.Fatalf("客户端超时应计失败并打开熔断，state=%d", state)
	}
	if err := client.Invoke(context.Background(), "/x", nil, &env, checkTestEnvelope); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("期望 ErrCircuitOpen，实际 %v", err)
	}
}

// TestInvoke_CanceledContextIsNeutral 验证首个尝试前取消不清零熔断窗口
func TestInvoke_CanceledContextIsNeutral(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL,
		WithMaxAttempts(1),
		WithBreaker(BreakerOptions{
			WindowSize: 2, MinRequests: 2, FailureRate: 1, SlowRate: 1,
			SlowThreshold: time.Minute, OpenDuration: time.Minute,
		}),
	)
	var env testEnvelope
	_ = client.Invoke(context.Background(), "/x", nil, &env, checkTestEnvelope)
	if _, size := breakerSnapshot(client); size != 1 {
		t.Fatalf("首次 503 后窗口长度=%d，期望 1", size)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := client.Invoke(canceled, "/x", nil, &env, checkTestEnvelope); !errors.Is(err, context.Canceled) {
		t.Fatalf("期望 context.Canceled，实际 %v", err)
	}
	if _, size := breakerSnapshot(client); size != 1 {
		t.Fatalf("取消请求不应写入熔断窗口，长度=%d", size)
	}

	_ = client.Invoke(context.Background(), "/x", nil, &env, checkTestEnvelope)
	if state, _ := breakerSnapshot(client); state != breakerOpen {
		t.Fatalf("第二次 503 后应打开熔断，state=%d", state)
	}
}

// TestInvoke_MidAttemptCancelIsNeutral 验证请求进行中 ctx deadline 不写入熔断窗口
func TestInvoke_MidAttemptCancelIsNeutral(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer bad.Close()

	client := newTestRPCClient(bad.URL,
		WithMaxAttempts(1),
		WithBreaker(BreakerOptions{
			WindowSize: 2, MinRequests: 2, FailureRate: 1, SlowRate: 1,
			SlowThreshold: time.Minute, OpenDuration: time.Minute,
		}),
	)
	var env testEnvelope
	_ = client.Invoke(context.Background(), "/x", nil, &env, checkTestEnvelope)

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte(`{"code":"OK"}`))
	}))
	defer slow.Close()

	client.httpClient = &http.Client{Timeout: time.Second}
	client.testResolve = func(context.Context, string) (string, error) { return slow.URL, nil }
	client.opts.MaxAttempts = 3

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := client.Invoke(ctx, "/x", nil, &env, checkTestEnvelope); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("期望 context.DeadlineExceeded，实际 %v", err)
	}
	if _, size := breakerSnapshot(client); size != 1 {
		t.Fatalf("请求进行中取消不应写入熔断窗口，长度=%d", size)
	}

	client.httpClient = &http.Client{Timeout: time.Second}
	client.testResolve = func(context.Context, string) (string, error) { return bad.URL, nil }
	client.opts.MaxAttempts = 1
	_ = client.Invoke(context.Background(), "/x", nil, &env, checkTestEnvelope)
	if state, _ := breakerSnapshot(client); state != breakerOpen {
		t.Fatalf("第二次 503 后应打开熔断，state=%d", state)
	}
}

// TestInvoke_SlowCallOpensBreaker 验证慢调用率达到阈值后打开熔断
func TestInvoke_SlowCallOpensBreaker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"OK"}`))
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL,
		WithHTTPTimeout(time.Second),
		WithBreaker(BreakerOptions{
			WindowSize: 2, MinRequests: 2, FailureRate: 1, SlowRate: 1,
			SlowThreshold: 10 * time.Millisecond, OpenDuration: time.Minute,
		}),
	)
	var env testEnvelope
	for i := 0; i < 2; i++ {
		if err := client.Invoke(context.Background(), "/x", nil, &env, checkTestEnvelope); err != nil {
			t.Fatalf("第 %d 次调用: %v", i+1, err)
		}
	}
	if state, _ := breakerSnapshot(client); state != breakerOpen {
		t.Fatalf("慢调用率达到阈值后应打开熔断，state=%d", state)
	}
	if err := client.Invoke(context.Background(), "/x", nil, &env, checkTestEnvelope); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("期望 ErrCircuitOpen，实际 %v", err)
	}
}

// TestInvoke_HookErrorDoesNotTripBreaker 验证业务 hook 错误不计熔断失败
func TestInvoke_HookErrorDoesNotTripBreaker(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"INVALID_INPUT","msg":"bad"}`))
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL,
		WithMaxAttempts(3),
		WithBreaker(BreakerOptions{
			WindowSize: 1, MinRequests: 1, FailureRate: 1, SlowRate: 1,
			SlowThreshold: time.Minute, OpenDuration: time.Minute,
		}),
	)
	var env testEnvelope
	for i := 0; i < 2; i++ {
		err := client.Invoke(context.Background(), "/x", nil, &env, checkTestEnvelope)
		var biz *errors.BizError
		if !errors.As(err, &biz) || biz.Code != "INVALID_INPUT" {
			t.Fatalf("第 %d 次期望 INVALID_INPUT，实际 %v", i+1, err)
		}
	}
	if hits.Load() != 2 {
		t.Fatalf("业务 hook 错误不应打开熔断，hits=%d", hits.Load())
	}
	if state, _ := breakerSnapshot(client); state != breakerClosed {
		t.Fatalf("业务 hook 错误不应改变熔断状态，state=%d", state)
	}
}

// TestNewHttpRpcClient_SharesBreakerByService 验证同一服务共享熔断器
func TestNewHttpRpcClient_SharesBreakerByService(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	name := newTestServiceName()
	cfg := BreakerOptions{
		WindowSize: 1, MinRequests: 1, FailureRate: 1, SlowRate: 1,
		SlowThreshold: time.Minute, OpenDuration: time.Minute,
	}
	resolve := func(context.Context, string) (string, error) { return server.URL, nil }
	c1 := NewHttpRpcClient(nil, name, WithMaxAttempts(1), WithBreaker(cfg))
	c1.testResolve = resolve
	c2 := NewHttpRpcClient(nil, name, WithMaxAttempts(1), WithBreaker(cfg))
	c2.testResolve = resolve
	if c1.breaker != c2.breaker {
		t.Fatal("同一 serviceName 应共享 breaker")
	}
	other := NewHttpRpcClient(nil, newTestServiceName(), WithMaxAttempts(1), WithBreaker(cfg))
	if other.breaker == c1.breaker {
		t.Fatal("不同 serviceName 不应共享 breaker")
	}

	var env testEnvelope
	if err := c1.Invoke(context.Background(), "/x", nil, &env, checkTestEnvelope); err == nil {
		t.Fatal("503 应返回错误")
	}
	if err := c2.Invoke(context.Background(), "/x", nil, &env, checkTestEnvelope); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("同服务第二个 client 应命中共享熔断，实际 %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("共享熔断后不应再次请求下游，hits=%d", hits.Load())
	}
}

// TestNewHttpRpcClient_BreakerConfigFirstWins 验证同一服务第一次创建的熔断配置生效
func TestNewHttpRpcClient_BreakerConfigFirstWins(t *testing.T) {
	name := newTestServiceName()
	first := BreakerOptions{
		WindowSize: 7, MinRequests: 7, FailureRate: 1, SlowRate: 1,
		SlowThreshold: time.Minute, OpenDuration: time.Minute,
	}
	second := BreakerOptions{
		WindowSize: 3, MinRequests: 3, FailureRate: 0.5, SlowRate: 0.5,
		SlowThreshold: time.Minute, OpenDuration: time.Minute,
	}
	c1 := NewHttpRpcClient(nil, name, WithBreaker(first))
	c2 := NewHttpRpcClient(nil, name, WithBreaker(second))
	if c1.breaker != c2.breaker {
		t.Fatal("同一 serviceName 应共享 breaker")
	}
	if c2.breaker.cfg.WindowSize != 7 || c2.breaker.cfg.MinRequests != 7 {
		t.Fatalf("应复用第一次配置，实际 window=%d min=%d", c2.breaker.cfg.WindowSize, c2.breaker.cfg.MinRequests)
	}
}

// marshalSpy 记录 json.Marshal 调用并返回错误
type marshalSpy struct {
	calls *atomic.Int64
}

// MarshalJSON 记录调用并返回错误
func (m marshalSpy) MarshalJSON() ([]byte, error) {
	m.calls.Add(1)
	return nil, errors.New("marshal failed")
}

// TestInvoke_OpenBreakerSkipsMarshal 验证熔断打开时不序列化请求
func TestInvoke_OpenBreakerSkipsMarshal(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL,
		WithMaxAttempts(1),
		WithBreaker(BreakerOptions{
			WindowSize: 1, MinRequests: 1, FailureRate: 1, SlowRate: 1,
			SlowThreshold: time.Minute, OpenDuration: time.Minute,
		}),
	)
	var env testEnvelope
	_ = client.Invoke(context.Background(), "/x", nil, &env, NoopBizHook)
	if state, _ := breakerSnapshot(client); state != breakerOpen {
		t.Fatalf("首次 503 后应打开熔断，state=%d", state)
	}

	var calls atomic.Int64
	err := client.Invoke(context.Background(), "/x", marshalSpy{calls: &calls}, &env, NoopBizHook)
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("期望 ErrCircuitOpen，实际 %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("熔断打开时不应序列化请求，calls=%d", calls.Load())
	}
	if hits.Load() != 1 {
		t.Fatalf("熔断打开时不应请求下游，hits=%d", hits.Load())
	}
}

// TestInvoke_MarshalFailureReleasesProbe 验证序列化失败释放半开探测位
func TestInvoke_MarshalFailureReleasesProbe(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL,
		WithMaxAttempts(1),
		WithBreaker(BreakerOptions{
			WindowSize: 1, MinRequests: 1, FailureRate: 1, SlowRate: 1,
			SlowThreshold: time.Minute, OpenDuration: 10 * time.Millisecond,
		}),
	)
	var env testEnvelope
	_ = client.Invoke(context.Background(), "/x", nil, &env, NoopBizHook)
	time.Sleep(20 * time.Millisecond)

	var calls atomic.Int64
	err := client.Invoke(context.Background(), "/x", marshalSpy{calls: &calls}, &env, NoopBizHook)
	if err == nil || errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("期望序列化错误，实际 %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("序列化应被调用一次，calls=%d", calls.Load())
	}
	if hits.Load() != 1 {
		t.Fatalf("序列化失败不应请求下游，hits=%d", hits.Load())
	}

	client.breaker.mu.Lock()
	state, probe := client.breaker.state, client.breaker.probeInFlight
	client.breaker.mu.Unlock()
	if state != breakerHalfOpen || probe {
		t.Fatalf("序列化失败应释放探测位，state=%d probe=%v", state, probe)
	}
}

// TestInvoke_NilBizHookRejected 验证 bizHook 必填
func TestInvoke_NilBizHookRejected(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"code":"OK"}`))
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL)
	if err := client.Invoke(context.Background(), "/x", nil, nil, nil); err == nil {
		t.Fatal("nil bizHook 应返回错误")
	}
	if hits.Load() != 0 {
		t.Fatalf("nil bizHook 不应发起请求，hits=%d", hits.Load())
	}
}

// TestInvoke_NoopBizHook 验证纯传输的显式入口
func TestInvoke_NoopBizHook(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":"INVALID_INPUT","msg":"bad"}`))
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL)
	var env testEnvelope
	if err := client.Invoke(context.Background(), "/x", nil, &env, NoopBizHook); err != nil {
		t.Fatalf("NoopBizHook 只做传输，实际 %v", err)
	}
	if env.Code != "INVALID_INPUT" {
		t.Fatalf("响应应已反序列化，code=%q", env.Code)
	}
}

// TestInvoke_429NotRetried 验证 429 不自动重试且暴露 Retry-After
func TestInvoke_429NotRetried(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL,
		WithMaxAttempts(3),
		WithBackoff(time.Millisecond, time.Millisecond),
		WithBreaker(BreakerOptions{
			WindowSize: 1, MinRequests: 1, FailureRate: 1, SlowRate: 1,
			SlowThreshold: time.Minute, OpenDuration: time.Minute,
		}),
	)
	var env testEnvelope
	err := client.Invoke(context.Background(), "/x", nil, &env, checkTestEnvelope)
	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) || statusErr.Code != http.StatusTooManyRequests {
		t.Fatalf("期望 HTTP 429，实际 %v", err)
	}
	if statusErr.RetryAfter != time.Minute {
		t.Fatalf("RetryAfter: got %v want 1m", statusErr.RetryAfter)
	}
	if hits.Load() != 1 {
		t.Fatalf("429 不应自动重试，hits=%d", hits.Load())
	}
	if state, _ := breakerSnapshot(client); state != breakerClosed {
		t.Fatalf("429 不应打开熔断，state=%d", state)
	}
}

// TestInvoke_HookRejectsWithoutError 验证 false+nil 返回 ErrBizRejected
func TestInvoke_HookRejectsWithoutError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":"OK"}`))
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL,
		WithBreaker(BreakerOptions{
			WindowSize: 1, MinRequests: 1, FailureRate: 1, SlowRate: 1,
			SlowThreshold: time.Minute, OpenDuration: time.Minute,
		}),
	)
	var env testEnvelope
	err := client.Invoke(context.Background(), "/x", nil, &env, func(context.Context, BizHookContext) (bool, error) {
		return false, nil
	})
	if !errors.Is(err, ErrBizRejected) {
		t.Fatalf("期望 ErrBizRejected，实际 %v", err)
	}
	if state, _ := breakerSnapshot(client); state != breakerClosed {
		t.Fatalf("业务拒绝不应打开熔断，state=%d", state)
	}
}
