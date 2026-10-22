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

// TestInvoke_AttemptTimeoutCountsAsFailure 验证单次尝试超时计入熔断失败；默认只尝试一次
func TestInvoke_AttemptTimeoutCountsAsFailure(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL,
		WithBreaker(BreakerOptions{
			WindowSize: 1, MinRequests: 1, FailureRate: 1, SlowRate: 1,
			SlowThreshold: time.Minute, OpenDuration: time.Minute,
		}),
	)
	var env testEnvelope
	err := client.Invoke(context.Background(), InvokeRequest{
		Endpoint: "/x",
		RespBody: &env,
		BizHook:  checkTestEnvelope,
		Timeout:  20 * time.Millisecond,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("期望 context.DeadlineExceeded，实际 %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("默认 MaxAttempts=1，hits=%d", hits.Load())
	}
	if state, _ := breakerSnapshot(client); state != breakerOpen {
		t.Fatalf("单次尝试超时应计熔断失败，state=%d", state)
	}
	if err := invokeTest(client, context.Background(), "/x", nil, &env, checkTestEnvelope); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("期望 ErrCircuitOpen，实际 %v", err)
	}
}

// TestInvoke_AttemptTimeoutStopsWhenBreakerOpens 验证单次超时打开熔断后不再重试
func TestInvoke_AttemptTimeoutStopsWhenBreakerOpens(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL,
		WithBackoff(time.Millisecond, time.Millisecond),
		WithBreaker(BreakerOptions{
			WindowSize: 1, MinRequests: 1, FailureRate: 1, SlowRate: 1,
			SlowThreshold: time.Minute, OpenDuration: time.Minute,
		}),
	)
	var env testEnvelope
	err := client.Invoke(context.Background(), InvokeRequest{
		Endpoint:    "/x",
		RespBody:    &env,
		BizHook:     checkTestEnvelope,
		Timeout:     20 * time.Millisecond,
		MaxAttempts: 3,
	})
	if err == nil {
		t.Fatal("期望超时错误")
	}
	if hits.Load() != 1 {
		t.Fatalf("熔断打开后不应继续重试，hits=%d", hits.Load())
	}
	if state, _ := breakerSnapshot(client); state != breakerOpen {
		t.Fatalf("超时应打开熔断，state=%d", state)
	}
}

// TestInvoke_AttemptTimeoutNoRetryInHalfOpen 验证半开探测超时也只尝试一次
func TestInvoke_AttemptTimeoutNoRetryInHalfOpen(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL,
		WithBackoff(time.Millisecond, time.Millisecond),
		WithBreaker(BreakerOptions{
			WindowSize: 1, MinRequests: 1, FailureRate: 1, SlowRate: 1,
			SlowThreshold: time.Minute, OpenDuration: 10 * time.Millisecond,
		}),
	)
	var env testEnvelope
	_ = invokeTestWith(client, context.Background(), "/x", nil, &env, checkTestEnvelope, 1)
	if state, _ := breakerSnapshot(client); state != breakerOpen {
		t.Fatalf("首次 500 应打开熔断，state=%d", state)
	}
	time.Sleep(30 * time.Millisecond)

	before := hits.Load()
	err := client.Invoke(context.Background(), InvokeRequest{
		Endpoint:    "/x",
		RespBody:    &env,
		BizHook:     checkTestEnvelope,
		Timeout:     20 * time.Millisecond,
		MaxAttempts: 3,
	})
	if err == nil {
		t.Fatal("半开探测超时应返回错误")
	}
	if got := hits.Load() - before; got != 1 {
		t.Fatalf("半开探测只应发一次，实际 %d", got)
	}
	if state, _ := breakerSnapshot(client); state != breakerOpen {
		t.Fatalf("探测超时应重新打开熔断，state=%d", state)
	}
}

// TestInvoke_CanceledContextIsNeutral 验证首个尝试前取消不清零熔断窗口
func TestInvoke_CanceledContextIsNeutral(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL,
		WithBreaker(BreakerOptions{
			WindowSize: 2, MinRequests: 2, FailureRate: 1, SlowRate: 1,
			SlowThreshold: time.Minute, OpenDuration: time.Minute,
		}),
	)
	var env testEnvelope
	_ = invokeTestWith(client, context.Background(), "/x", nil, &env, checkTestEnvelope, 1)
	if _, size := breakerSnapshot(client); size != 1 {
		t.Fatalf("首次 503 后窗口长度=%d，期望 1", size)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := invokeTest(client, canceled, "/x", nil, &env, checkTestEnvelope); !errors.Is(err, context.Canceled) {
		t.Fatalf("期望 context.Canceled，实际 %v", err)
	}
	if _, size := breakerSnapshot(client); size != 1 {
		t.Fatalf("取消请求不应写入熔断窗口，长度=%d", size)
	}

	_ = invokeTestWith(client, context.Background(), "/x", nil, &env, checkTestEnvelope, 1)
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
		WithBreaker(BreakerOptions{
			WindowSize: 2, MinRequests: 2, FailureRate: 1, SlowRate: 1,
			SlowThreshold: time.Minute, OpenDuration: time.Minute,
		}),
	)
	var env testEnvelope
	_ = invokeTestWith(client, context.Background(), "/x", nil, &env, checkTestEnvelope, 1)

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte(`{"code":"OK"}`))
	}))
	defer slow.Close()

	client.httpClient = &http.Client{}
	client.resolver = resolverFunc(func(context.Context, string, string) (string, error) { return slow.URL, nil })

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := invokeTest(client, ctx, "/x", nil, &env, checkTestEnvelope); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("期望 context.DeadlineExceeded，实际 %v", err)
	}
	if _, size := breakerSnapshot(client); size != 1 {
		t.Fatalf("请求进行中取消不应写入熔断窗口，长度=%d", size)
	}

	client.httpClient = &http.Client{}
	client.resolver = resolverFunc(func(context.Context, string, string) (string, error) { return bad.URL, nil })
	_ = invokeTestWith(client, context.Background(), "/x", nil, &env, checkTestEnvelope, 1)
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
		WithBreaker(BreakerOptions{
			WindowSize: 2, MinRequests: 2, FailureRate: 1, SlowRate: 1,
			SlowThreshold: 10 * time.Millisecond, OpenDuration: time.Minute,
		}),
	)
	var env testEnvelope
	for i := 0; i < 2; i++ {
		if err := invokeTest(client, context.Background(), "/x", nil, &env, checkTestEnvelope); err != nil {
			t.Fatalf("第 %d 次调用: %v", i+1, err)
		}
	}
	if state, _ := breakerSnapshot(client); state != breakerOpen {
		t.Fatalf("慢调用率达到阈值后应打开熔断，state=%d", state)
	}
	if err := invokeTest(client, context.Background(), "/x", nil, &env, checkTestEnvelope); !errors.Is(err, ErrCircuitOpen) {
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
		WithBreaker(BreakerOptions{
			WindowSize: 1, MinRequests: 1, FailureRate: 1, SlowRate: 1,
			SlowThreshold: time.Minute, OpenDuration: time.Minute,
		}),
	)
	var env testEnvelope
	for i := 0; i < 2; i++ {
		err := invokeTest(client, context.Background(), "/x", nil, &env, checkTestEnvelope)
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

// TestNewHttpRpcClient_IndependentBreakers 验证每个 client 的熔断器互相独立
func TestNewHttpRpcClient_IndependentBreakers(t *testing.T) {
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
	resolve := resolverFunc(func(context.Context, string, string) (string, error) { return server.URL, nil })
	c1 := NewHttpRpcClient(resolve, name, WithBreaker(cfg))
	c2 := NewHttpRpcClient(resolve, name, WithBreaker(cfg))
	if c1.breaker == c2.breaker {
		t.Fatal("每个 client 应有独立 breaker")
	}

	var env testEnvelope
	if err := invokeTestWith(c1, context.Background(), "/x", nil, &env, checkTestEnvelope, 1); err == nil {
		t.Fatal("503 应返回错误")
	}
	if state, _ := breakerSnapshot(c1); state != breakerOpen {
		t.Fatalf("c1 的熔断应打开，state=%d", state)
	}
	if err := invokeTestWith(c2, context.Background(), "/x", nil, &env, checkTestEnvelope, 1); errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("c2 不应受 c1 熔断影响，实际 %v", err)
	}
	if hits.Load() != 2 {
		t.Fatalf("两个 client 应各自请求下游，hits=%d", hits.Load())
	}
}

// TestNewHttpRpcClient_BreakerOptionsIndependent 验证每个 client 使用自己的熔断配置
func TestNewHttpRpcClient_BreakerOptionsIndependent(t *testing.T) {
	name := newTestServiceName()
	first := BreakerOptions{
		WindowSize: 7, MinRequests: 7, FailureRate: 1, SlowRate: 1,
		SlowThreshold: time.Minute, OpenDuration: time.Minute,
	}
	second := BreakerOptions{
		WindowSize: 3, MinRequests: 3, FailureRate: 0.5, SlowRate: 0.5,
		SlowThreshold: time.Minute, OpenDuration: time.Minute,
	}
	resolve := resolverFunc(func(context.Context, string, string) (string, error) {
		return "http://example.test", nil
	})
	c1 := NewHttpRpcClient(resolve, name, WithBreaker(first))
	c2 := NewHttpRpcClient(resolve, name, WithBreaker(second))
	if c1.breaker == c2.breaker {
		t.Fatal("每个 client 应有独立 breaker")
	}
	if c1.breaker.cfg.WindowSize != 7 || c1.breaker.cfg.MinRequests != 7 {
		t.Fatalf("c1 配置不应被覆盖，实际 window=%d min=%d", c1.breaker.cfg.WindowSize, c1.breaker.cfg.MinRequests)
	}
	if c2.breaker.cfg.WindowSize != 3 || c2.breaker.cfg.MinRequests != 3 {
		t.Fatalf("c2 配置不应被覆盖，实际 window=%d min=%d", c2.breaker.cfg.WindowSize, c2.breaker.cfg.MinRequests)
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
		WithBreaker(BreakerOptions{
			WindowSize: 1, MinRequests: 1, FailureRate: 1, SlowRate: 1,
			SlowThreshold: time.Minute, OpenDuration: time.Minute,
		}),
	)
	var env testEnvelope
	_ = invokeTestWith(client, context.Background(), "/x", nil, &env, NoopBizHook, 1)
	if state, _ := breakerSnapshot(client); state != breakerOpen {
		t.Fatalf("首次 503 后应打开熔断，state=%d", state)
	}

	var calls atomic.Int64
	err := invokeTest(client, context.Background(), "/x", marshalSpy{calls: &calls}, &env, NoopBizHook)
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
		WithBreaker(BreakerOptions{
			WindowSize: 1, MinRequests: 1, FailureRate: 1, SlowRate: 1,
			SlowThreshold: time.Minute, OpenDuration: 10 * time.Millisecond,
		}),
	)
	var env testEnvelope
	_ = invokeTestWith(client, context.Background(), "/x", nil, &env, NoopBizHook, 1)
	time.Sleep(20 * time.Millisecond)

	var calls atomic.Int64
	err := invokeTest(client, context.Background(), "/x", marshalSpy{calls: &calls}, &env, NoopBizHook)
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
	if err := invokeTest(client, context.Background(), "/x", nil, nil, nil); err == nil {
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
	if err := invokeTest(client, context.Background(), "/x", nil, &env, NoopBizHook); err != nil {
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
		WithBackoff(time.Millisecond, time.Millisecond),
		WithBreaker(BreakerOptions{
			WindowSize: 1, MinRequests: 1, FailureRate: 1, SlowRate: 1,
			SlowThreshold: time.Minute, OpenDuration: time.Minute,
		}),
	)
	var env testEnvelope
	err := invokeTest(client, context.Background(), "/x", nil, &env, checkTestEnvelope)
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
	err := invokeTest(client, context.Background(), "/x", nil, &env, func(context.Context, BizHookContext) (bool, error) {
		return false, nil
	})
	if !errors.Is(err, ErrBizRejected) {
		t.Fatalf("期望 ErrBizRejected，实际 %v", err)
	}
	if state, _ := breakerSnapshot(client); state != breakerClosed {
		t.Fatalf("业务拒绝不应打开熔断，state=%d", state)
	}
}

// TestInvoke_BreakerOpenStopsRetry 验证本次尝试打开熔断后不再发剩余重试
func TestInvoke_BreakerOpenStopsRetry(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL,
		WithBackoff(time.Millisecond, time.Millisecond),
		WithBreaker(BreakerOptions{
			WindowSize: 1, MinRequests: 1, FailureRate: 1, SlowRate: 1,
			SlowThreshold: time.Minute, OpenDuration: time.Minute,
		}),
	)
	var env testEnvelope
	err := client.Invoke(context.Background(), InvokeRequest{
		Endpoint: "/x", RespBody: &env, BizHook: checkTestEnvelope, MaxAttempts: 5,
	})
	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) || statusErr.Code != http.StatusServiceUnavailable {
		t.Fatalf("期望 HTTP 503，实际 %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("熔断打开后不应继续重试，hits=%d", hits.Load())
	}
	if err := invokeTest(client, context.Background(), "/x", nil, &env, checkTestEnvelope); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("期望 ErrCircuitOpen，实际 %v", err)
	}
}

// TestInvoke_HalfOpenProbeSingleAttempt 验证半开探测一次调用只发一个请求
func TestInvoke_HalfOpenProbeSingleAttempt(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL,
		WithBackoff(time.Millisecond, time.Millisecond),
		WithBreaker(BreakerOptions{
			WindowSize: 1, MinRequests: 1, FailureRate: 1, SlowRate: 1,
			SlowThreshold: time.Minute, OpenDuration: 10 * time.Millisecond,
		}),
	)
	var env testEnvelope
	_ = invokeTestWith(client, context.Background(), "/x", nil, &env, checkTestEnvelope, 1)
	time.Sleep(20 * time.Millisecond)

	before := hits.Load()
	err := client.Invoke(context.Background(), InvokeRequest{
		Endpoint: "/x", RespBody: &env, BizHook: checkTestEnvelope, MaxAttempts: 5,
	})
	if err == nil {
		t.Fatal("探测 503 应返回错误")
	}
	if got := hits.Load() - before; got != 1 {
		t.Fatalf("半开探测一次调用应只发一个请求，实际 %d", got)
	}
}

// panicBody 在反序列化时 panic，用于验证半开探测位兜底释放
type panicBody struct{}

// UnmarshalJSON 故意 panic
func (*panicBody) UnmarshalJSON([]byte) error { panic("boom") }

// TestInvoke_PanicInUnmarshalReleasesProbe 验证调用方代码 panic 后半开探测位被释放
func TestInvoke_PanicInUnmarshalReleasesProbe(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"OK"}`))
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL,
		WithBreaker(BreakerOptions{
			WindowSize: 1, MinRequests: 1, FailureRate: 1, SlowRate: 1,
			SlowThreshold: time.Minute, OpenDuration: 10 * time.Millisecond,
		}),
	)
	var env testEnvelope
	_ = invokeTestWith(client, context.Background(), "/x", nil, &env, checkTestEnvelope, 1)
	time.Sleep(20 * time.Millisecond)

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("期望反序列化 panic")
			}
		}()
		_ = invokeTestWith(client, context.Background(), "/x", nil, &panicBody{}, checkTestEnvelope, 1)
	}()

	if err := invokeTest(client, context.Background(), "/x", nil, &env, checkTestEnvelope); err != nil {
		t.Fatalf("panic 后探测位应已释放，实际 %v", err)
	}
}

// TestInvoke_HalfOpenProbeNoRetryOnDiscoveryError 验证半开探测遇到发现错误不重试
func TestInvoke_HalfOpenProbeNoRetryOnDiscoveryError(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"OK"}`))
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL,
		WithBreaker(BreakerOptions{
			WindowSize: 1, MinRequests: 1, FailureRate: 1, SlowRate: 1,
			SlowThreshold: time.Minute, OpenDuration: 10 * time.Millisecond,
		}),
	)
	var env testEnvelope
	_ = invokeTestWith(client, context.Background(), "/x", nil, &env, checkTestEnvelope, 1)
	time.Sleep(20 * time.Millisecond)

	var resolveCalls atomic.Int64
	client.resolver = resolverFunc(func(context.Context, string, string) (string, error) {
		resolveCalls.Add(1)
		return "", errors.New("consul down")
	})
	err := client.Invoke(context.Background(), InvokeRequest{
		Endpoint: "/x", RespBody: &env, BizHook: checkTestEnvelope, MaxAttempts: 5,
	})
	var discoveryErr *discoveryError
	if !errors.As(err, &discoveryErr) {
		t.Fatalf("期望发现错误，实际 %v", err)
	}
	if resolveCalls.Load() != 1 {
		t.Fatalf("半开探测不应重试发现，resolver 调用 %d 次", resolveCalls.Load())
	}

	client.resolver = resolverFunc(func(context.Context, string, string) (string, error) {
		return server.URL, nil
	})
	if err := invokeTest(client, context.Background(), "/x", nil, &env, checkTestEnvelope); err != nil {
		t.Fatalf("探测位释放后下一次调用应成功: %v", err)
	}
}

// TestInvoke_RedirectPolicyErrorNotRetried 验证自定义重定向策略拒绝时不重试、不计熔断
func TestInvoke_RedirectPolicyErrorNotRetried(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/other", http.StatusFound)
			return
		}
		http.Redirect(w, r, "/", http.StatusFound)
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL, WithCheckRedirect(func(_ *http.Request, via []*http.Request) error {
		if len(via) >= 2 {
			return errors.New("redirect cap")
		}
		return nil
	}))
	var env testEnvelope
	err := client.Invoke(context.Background(), InvokeRequest{
		Endpoint: "/x", RespBody: &env, BizHook: checkTestEnvelope, MaxAttempts: 3,
	})
	var reqErr *requestError
	if !errors.As(err, &reqErr) {
		t.Fatalf("期望 requestError，实际 %v", err)
	}
	if hits.Load() != 2 {
		t.Fatalf("重定向策略拒绝不应重试，hits=%d want 2", hits.Load())
	}
	if state, size := breakerSnapshot(client); state != breakerClosed || size != 0 {
		t.Fatalf("不应计入熔断，state=%d window=%d", state, size)
	}
}
