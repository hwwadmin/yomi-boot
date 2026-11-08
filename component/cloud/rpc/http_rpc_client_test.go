package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const maxInt64JSON = 9223372036854775807

// TestDefaultRequestTimeout 验证默认请求超时为 3 秒
func TestDefaultRequestTimeout(t *testing.T) {
	if defaultRequestTimeout != 3*time.Second {
		t.Fatalf("defaultRequestTimeout: got %v want 3s", defaultRequestTimeout)
	}
}

// TestInvoke_PreservesInt64Precision 验证大整数经 RPC 响应解析后不丢失精度
func TestInvoke_PreservesInt64Precision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"OK","msg":"","data":{"id":9223372036854775807}}`))
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL)
	var env testEnvelope
	if err := invokeTest(client, context.Background(), "/x", nil, &env); err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	var resp struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(env.Data, &resp); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if resp.ID != maxInt64JSON {
		t.Fatalf("id: got %d, want %d", resp.ID, maxInt64JSON)
	}
}

// TestInvoke_PreservesInt64InPageResponse 验证分页响应中 total 与 data 的大整数均不丢失精度
func TestInvoke_PreservesInt64InPageResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"OK","msg":"","data":{"items":[{"id":9223372036854775807}],"total":9223372036854775807}}`))
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL)
	var env testEnvelope
	if err := invokeTest(client, context.Background(), "/x", nil, &env); err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	var page struct {
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(env.Data, &page); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if page.Total != maxInt64JSON {
		t.Fatalf("total: got %d, want %d", page.Total, maxInt64JSON)
	}
	if len(page.Items) != 1 || page.Items[0].ID != maxInt64JSON {
		t.Fatalf("items: got %+v, want one item with id %d", page.Items, maxInt64JSON)
	}
}

// TestNewHttpRpcClient_Validation 验证构造函数对空 resolver/空服务名 Fail Fast
func TestNewHttpRpcClient_Validation(t *testing.T) {
	assertPanic := func(fn func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Fatal("期望 panic")
			}
		}()
		fn()
	}

	assertPanic(func() {
		NewHttpRpcClient(nil, "svc")
	})
	assertPanic(func() {
		NewHttpRpcClient(resolverFunc(func(context.Context, string, string) (string, error) {
			return "http://example.test", nil
		}), "  ")
	})

	client := NewHttpRpcClient(resolverFunc(func(context.Context, string, string) (string, error) {
		return "http://example.test", nil
	}), "svc")
	if client == nil {
		t.Fatal("合法参数应返回 client")
	}
}

func TestLogHelpersAndNormalize(t *testing.T) {
	client := &HttpRpcClient{serviceName: "demo"}
	ctx := context.Background()
	client.logRequest(ctx, "/ping", nil)
	client.logRequest(ctx, "/ping", bytes.Repeat([]byte("a"), defaultMaxLogBodyBytes+8))
	client.logResponse(ctx, "/ping", 1, 0, []byte("ok"))
	client.logResponse(ctx, "/ping", 2, http.StatusOK, []byte("ok"))
	statusErr := &HTTPStatusError{Code: http.StatusServiceUnavailable, Detail: "down", RetryAfter: time.Second}
	client.logRetry(ctx, "/ping", 1, 3, time.Millisecond, statusErr)
	client.logBudgetExhausted(ctx, "/ping", 3, 3, time.Millisecond, context.DeadlineExceeded)
	client.logBreakerOpen(ctx, "/ping", 1, "open", statusErr)
	client.logBreakerReject(ctx, "/ping", 1)
	client.logBreakerProbe(ctx, "/ping", 1)
	client.logRetryRecovered(ctx, "/ping", 2)

	if statusCodeOf(errors.New("x")) != 0 || statusCodeOf(statusErr) != http.StatusServiceUnavailable {
		t.Fatal("statusCodeOf 结果不正确")
	}
	if truncateLogBody(nil) != "" {
		t.Fatal("空日志体应为空字符串")
	}
	for _, err := range []error{
		nil,
		context.Canceled,
		context.DeadlineExceeded,
		statusErr,
		&ProtocolError{Reason: "bad"},
		&discoveryError{err: errors.New("d")},
		&retryableError{err: errors.New("t")},
		errors.New("other"),
	} {
		_ = reasonOf(err)
	}

	normalized := normalizeHTTPClientOptions(httpClientOptions{MaxBackoff: time.Millisecond})
	if normalized.BaseBackoff <= 0 || normalized.MaxBackoff < normalized.BaseBackoff || normalized.Transport == nil || normalized.CheckRedirect == nil {
		t.Fatalf("客户端参数未归一化: %+v", normalized)
	}
	breaker := normalizeBreakerOptions(BreakerOptions{MinRequests: 20, WindowSize: 2, FailureRate: 2, SlowRate: -1})
	if breaker.WindowSize < breaker.MinRequests || breaker.FailureRate <= 0 || breaker.FailureRate > 1 || breaker.SlowRate <= 0 || breaker.OpenDuration <= 0 {
		t.Fatalf("熔断参数未归一化: %+v", breaker)
	}
	applied := applyHTTPClientOptions([]ClientOption{nil, WithLogBodies(true)})
	if !applied.LogBodies {
		t.Fatal("WithLogBodies 应打开请求体日志")
	}
}
