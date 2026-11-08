package rpc

import (
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
