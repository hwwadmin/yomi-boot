package rpc

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testRPCClient(resolver EndpointResolver, transport http.RoundTripper) *HttpRpcClient {
	client := NewHttpRpcClient(resolver, "svc", WithTransport(transport), WithLogBodies(true), WithBackoff(time.Millisecond, 2*time.Millisecond))
	return client
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestInvoke_RejectsNilClientCanceledCtxAndEmptyService(t *testing.T) {
	if err := (*HttpRpcClient)(nil).Invoke(context.Background(), InvokeRequest{}); err == nil {
		t.Fatal("未初始化客户端应失败")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := testRPCClient(resolverFunc(func(context.Context, string, string) (string, error) {
		return "http://example", nil
	}), nil)
	if err := client.Invoke(ctx, InvokeRequest{}); err == nil {
		t.Fatal("已取消的上下文应失败")
	}
	client.serviceName = ""
	if err := client.Invoke(context.Background(), InvokeRequest{}); err == nil {
		t.Fatal("空服务名应失败")
	}
}

func TestInvoke_RejectsUnmarshalableBody(t *testing.T) {
	client := testRPCClient(resolverFunc(func(context.Context, string, string) (string, error) {
		return "http://example", nil
	}), roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("序列化失败时不应发出请求")
		return nil, nil
	}))
	if err := client.Invoke(context.Background(), InvokeRequest{ReqBody: make(chan int)}); err == nil {
		t.Fatal("不可序列化的请求体应失败")
	}
}

func TestInvoke_LogsBodiesAndBadJSON(t *testing.T) {
	client := testRPCClient(resolverFunc(func(context.Context, string, string) (string, error) {
		return "http://example/ok", nil
	}), roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"ok":true}`), nil
	}))
	var out map[string]bool
	if err := client.Invoke(context.Background(), InvokeRequest{Endpoint: "/ok", ReqBody: map[string]string{"a": "b"}, RespBody: &out}); err != nil {
		t.Fatal(err)
	}

	client.httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusBadRequest, "no"), nil
	})
	if err := client.Invoke(context.Background(), InvokeRequest{Endpoint: "/bad", RespBody: &out}); err == nil {
		t.Fatal("400 应失败")
	}

	client.httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, "not-json"), nil
	})
	if err := client.Invoke(context.Background(), InvokeRequest{Endpoint: "/bad-json", RespBody: &out}); err == nil {
		t.Fatal("非法 JSON 应失败")
	}
}

func TestInvoke_DiscoveryCancelAndRetryThenBreaker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := testRPCClient(resolverFunc(func(context.Context, string, string) (string, error) {
		cancel()
		return "", errors.New("discover down")
	}), nil)
	if err := client.Invoke(ctx, InvokeRequest{Endpoint: "/x", MaxAttempts: 2}); err == nil {
		t.Fatal("发现阶段上下文取消应失败")
	}

	var calls int
	client = testRPCClient(resolverFunc(func(context.Context, string, string) (string, error) {
		calls++
		client.breaker.mu.Lock()
		client.breaker.state = breakerOpen
		client.breaker.openUntil = time.Now().Add(time.Minute)
		client.breaker.mu.Unlock()
		return "", errors.New("discover down")
	}), nil)
	client.opts.BaseBackoff = 0
	client.opts.MaxBackoff = 0
	if err := client.Invoke(context.Background(), InvokeRequest{Endpoint: "/x", MaxAttempts: 2}); err == nil {
		t.Fatal("重试时熔断打开应失败")
	}
	if calls != 1 {
		t.Fatalf("熔断打开后不应再次发现: %d", calls)
	}
}

func TestInvoke_ProbeReturnsTransportError(t *testing.T) {
	client := testRPCClient(resolverFunc(func(context.Context, string, string) (string, error) {
		return "http://example/probe", nil
	}), roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusBadRequest, "nope"), nil
	}))
	client.breaker.state = breakerHalfOpen
	if err := client.Invoke(context.Background(), InvokeRequest{Endpoint: "/probe"}); err == nil {
		t.Fatal("半开探测遇到 4xx 应返回该错误")
	}
}

func TestInvoke_RetrySleepCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := testRPCClient(resolverFunc(func(context.Context, string, string) (string, error) {
		return "http://example/retry", nil
	}), roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusServiceUnavailable, "later"), nil
	}))
	client.opts.BaseBackoff = 150 * time.Millisecond
	client.opts.MaxBackoff = 150 * time.Millisecond
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	if err := client.Invoke(ctx, InvokeRequest{Endpoint: "/retry", MaxAttempts: 2}); err == nil {
		t.Fatal("退避期间取消应失败")
	}
}

func TestDoRequest_RejectsBadURLAndTransport(t *testing.T) {
	client := testRPCClient(resolverFunc(func(context.Context, string, string) (string, error) {
		return "http://example", nil
	}), roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusFound, ""), errors.New("redirect")
	}))
	ctx := context.Background()
	if _, err := client.doRequest(ctx, " ", []byte("{}"), true, 32); err == nil {
		t.Fatal("空地址应失败")
	}
	if _, err := client.doRequest(ctx, "http://[", []byte("{}"), true, 32); err == nil {
		t.Fatal("非法 URL 应失败")
	}
	if _, err := client.doRequest(ctx, "http://example", []byte("{}"), true, 32); err == nil {
		t.Fatal("重定向被拒绝时应失败")
	}
	client.httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial")
	})
	if _, err := client.doRequest(ctx, "http://example", []byte("{}"), true, 32); err == nil {
		t.Fatal("网络失败应可重试")
	}
}

func TestDoRequest_BodyEdges(t *testing.T) {
	ctx := context.Background()
	client := testRPCClient(resolverFunc(func(context.Context, string, string) (string, error) {
		return "http://example", nil
	}), roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"ok":true}`), nil
	}))
	if _, err := client.doRequest(ctx, "http://example", []byte("{}"), true, 0); err != nil {
		t.Fatal(err)
	}

	client.httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(errBody{}), Header: make(http.Header)}, nil
	})
	if _, err := client.doRequest(ctx, "http://example", []byte("{}"), true, 32); err == nil {
		t.Fatal("读响应失败应返回错误")
	}

	client.httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusInternalServerError, "abcdefghij"), nil
	})
	if _, err := client.doRequest(ctx, "http://example", []byte("{}"), true, 4); err == nil {
		t.Fatal("超限的错误响应应失败")
	}

	client.httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusBadRequest, "   "), nil
	})
	if _, err := client.doRequest(ctx, "http://example", []byte("{}"), true, 64); err == nil {
		t.Fatal("空白错误响应应失败")
	}

	client.httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusBadRequest, strings.Repeat("y", 250)), nil
	})
	if _, err := client.doRequest(ctx, "http://example", []byte("{}"), true, 300); err == nil {
		t.Fatal("过长错误详情应失败")
	}

	drainBody(nil)
	if got := parseRetryAfter("99999999"); got != 24*time.Hour {
		t.Fatalf("超长 Retry-After = %s", got)
	}
	later := time.Now().Add(48 * time.Hour).UTC().Format(http.TimeFormat)
	if got := parseRetryAfter(later); got != 24*time.Hour {
		t.Fatalf("超长日期 Retry-After = %s", got)
	}
}

type errBody struct{}

func (errBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
