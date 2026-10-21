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
