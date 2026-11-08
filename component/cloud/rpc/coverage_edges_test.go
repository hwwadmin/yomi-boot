package rpc

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestErrorTypes(t *testing.T) {
	if (circuitOpenError{}).Error() != "CIRCUIT_OPEN" || (circuitOpenError{}).Unwrap() != ErrDownstream {
		t.Fatal("熔断错误应展开到下游哨兵")
	}

	var status *HTTPStatusError
	if status.Error() == "" || status.StatusCode() != 0 || status.Retryable() {
		t.Fatal("nil HTTP 状态错误边界不正确")
	}
	status = &HTTPStatusError{Code: http.StatusBadGateway, Detail: "bad", RetryAfter: time.Second}
	if status.Error() == "" || !status.Retryable() || status.Unwrap() != ErrDownstream || status.StatusCode() != http.StatusBadGateway {
		t.Fatalf("HTTP 状态错误 = %v", status)
	}
	if (&HTTPStatusError{Code: http.StatusTeapot}).Retryable() {
		t.Fatal("418 不应重试")
	}

	var protocol *ProtocolError
	if protocol.Error() == "" || protocol.Unwrap() != nil || !protocol.Is(ErrProtocol) {
		t.Fatal("nil 协议错误边界不正确")
	}
	protocol = &ProtocolError{Reason: "bad json"}
	if protocol.Error() != "bad json" {
		t.Fatalf("无底层错误的协议错误 = %q", protocol.Error())
	}
	protocol.Err = errors.New("eof")
	if protocol.Unwrap() == nil || protocol.Error() == "" {
		t.Fatal("协议错误应包含底层错误")
	}

	var retry *retryableError
	if retry.Error() == "" || retry.Unwrap() != nil || !retry.Is(ErrTransport) {
		t.Fatal("nil 可重试错误边界不正确")
	}
	retry = &retryableError{err: errors.New("reset")}
	if retry.Error() != "reset" {
		t.Fatalf("retryable = %q", retry.Error())
	}
	if wrapRetryable(nil) != nil {
		t.Fatal("包装 nil 应返回 nil")
	}

	var discovery *discoveryError
	if discovery.Error() == "" || discovery.Unwrap() != nil || !discovery.Is(ErrDiscovery) {
		t.Fatal("nil 发现错误边界不正确")
	}
	discovery = &discoveryError{err: errors.New("consul")}
	if discovery.Error() != "consul" {
		t.Fatalf("discovery = %q", discovery.Error())
	}

	var request *requestError
	if request.Error() == "" || request.Unwrap() != nil || !request.Is(ErrRequest) {
		t.Fatal("nil 请求错误边界不正确")
	}
	request = &requestError{err: errors.New("build")}
	if request.Error() != "build" {
		t.Fatalf("request = %q", request.Error())
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

func TestMemoryResolverAndRetryEdges(t *testing.T) {
	resolver := NewMemoryEndpointResolver(map[string]string{" ": "http://127.0.0.1", "demo": "http://127.0.0.1:8080/"})
	if _, err := (*MemoryEndpointResolver)(nil).ResolveHTTPBase(context.Background(), "demo", "/a"); err == nil {
		t.Fatal("nil 解析器应失败")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := resolver.ResolveHTTPBase(canceled, "demo", "/a"); err == nil {
		t.Fatal("已取消的 ctx 应失败")
	}
	if _, err := resolver.ResolveHTTPBase(context.Background(), " ", "/a"); err == nil {
		t.Fatal("空服务名应失败")
	}

	if backoffBase(0, time.Second, 10*time.Second) != time.Second {
		t.Fatal("attempt 小于 1 应按 1 计算")
	}
	if backoffBase(8, time.Second, 3*time.Second) != 3*time.Second {
		t.Fatal("退避应被上限截断")
	}
	if got := jitterBackoff(0); got != 0 {
		t.Fatalf("零退避 = %v", got)
	}
	if got := jitterBackoff(time.Nanosecond); got != time.Nanosecond {
		t.Fatalf("极小退避 = %v", got)
	}
	if !canWait(context.Background(), 0) || !canWait(context.Background(), time.Second) {
		t.Fatal("无截止时间或零等待应可以等待")
	}
	short, cancelShort := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancelShort()
	if canWait(short, time.Hour) {
		t.Fatal("预算不足时不应继续等待")
	}
	if err := sleepBackoff(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if err := sleepBackoff(canceled, time.Hour); err == nil {
		t.Fatal("已取消的等待应返回错误")
	}
}

func TestBreakerNilAndPlainHTTPError(t *testing.T) {
	var breaker *circuitBreaker
	if _, err := breaker.allow(); err != nil {
		t.Fatal(err)
	}
	if breaker.recordOutcome(breakerTicket{}, attemptResult{}) != breakerRecordClosed {
		t.Fatal("nil 熔断器记录应视为关闭")
	}
	breaker.onNeutral(breakerTicket{})
	if breaker.slowThreshold() != defaultBreakerSlowThreshold {
		t.Fatal("nil 熔断器应使用默认慢调用阈值")
	}
	if rate(nil, func(attemptResult) bool { return true }) != 0 {
		t.Fatal("空窗口比率应为 0")
	}

	plain := &HTTPStatusError{Code: http.StatusBadRequest, Detail: "bad"}
	if plain.Error() == "" {
		t.Fatal("没有 Retry-After 的状态错误应仍有描述")
	}
	if got := normalizeHTTPClientOptions(httpClientOptions{BaseBackoff: time.Second, MaxBackoff: 0}); got.MaxBackoff <= 0 {
		t.Fatalf("MaxBackoff = %v", got.MaxBackoff)
	}

	live := newCircuitBreaker(BreakerOptions{WindowSize: 2, MinRequests: 2})
	if live.recordOutcome(breakerTicket{probe: true, seq: 99}, attemptResult{}) != breakerRecordClosed {
		t.Fatal("过期探测票据不应打开熔断")
	}
}
