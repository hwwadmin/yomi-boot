// @author zwei

package rpc

import (
	"fmt"
	"net/http"

	"gitee.com/hq_project/yomi-boot/boot/errors"
)

// ErrDownstream 下游服务异常
var ErrDownstream = errors.NewNoStack("DOWNSTREAM_ERROR")

// circuitOpenError 熔断打开；Error 为 CIRCUIT_OPEN，Unwrap 到 ErrDownstream 以支持 errors.Is
type circuitOpenError struct{}

// Error 返回熔断打开错误码
func (circuitOpenError) Error() string { return "CIRCUIT_OPEN" }

// Unwrap 返回下游异常哨兵
func (circuitOpenError) Unwrap() error { return ErrDownstream }

// ErrCircuitOpen 下游熔断已打开（errors.Is 仍匹配 ErrDownstream）
var ErrCircuitOpen error = circuitOpenError{}

// HTTPStatusError 下游返回非 200 状态
type HTTPStatusError struct {
	Code   int    // Code 是 HTTP 状态码
	Detail string // Detail 是截断后的响应体或状态描述
}

// Error 返回 HTTP 状态错误描述
func (e *HTTPStatusError) Error() string {
	if e == nil {
		return "rpc: http status error"
	}
	return fmt.Sprintf("下游 HTTP %d: %s", e.Code, e.Detail)
}

// Unwrap 返回下游异常哨兵
func (e *HTTPStatusError) Unwrap() error { return ErrDownstream }

// StatusCode 返回 HTTP 状态码
func (e *HTTPStatusError) StatusCode() int {
	if e == nil {
		return 0
	}
	return e.Code
}

// Retryable 判断当前状态码是否可重试
func (e *HTTPStatusError) Retryable() bool {
	if e == nil {
		return false
	}
	switch e.Code {
	case http.StatusRequestTimeout, http.StatusTooManyRequests,
		http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// ProtocolError 传输成功但响应无法按约定解析
type ProtocolError struct {
	Reason string // Reason 是协议失败原因
	Err    error  // Err 是底层解析错误
}

// Error 返回协议错误描述
func (e *ProtocolError) Error() string {
	if e == nil {
		return "rpc: protocol error"
	}
	if e.Err == nil {
		return e.Reason
	}
	return fmt.Sprintf("%s: %v", e.Reason, e.Err)
}

// Unwrap 返回底层解析错误
func (e *ProtocolError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// retryableError 标记可重试的传输错误
type retryableError struct {
	err error
}

// Error 返回可重试传输错误信息
func (e *retryableError) Error() string {
	if e == nil || e.err == nil {
		return "rpc: retryable error"
	}
	return e.err.Error()
}

// Unwrap 返回底层错误
func (e *retryableError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// discoveryError 标记服务发现错误；可重试，但不计入下游熔断
type discoveryError struct {
	err error
}

// Error 返回服务发现错误信息
func (e *discoveryError) Error() string {
	if e == nil || e.err == nil {
		return "rpc: discovery error"
	}
	return e.err.Error()
}

// Unwrap 返回底层错误
func (e *discoveryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// requestError 标记本地请求构建错误；不可重试，也不计入熔断
type requestError struct {
	err error
}

// Error 返回本地请求错误信息
func (e *requestError) Error() string {
	if e == nil || e.err == nil {
		return "rpc: request error"
	}
	return e.err.Error()
}

// Unwrap 返回底层错误
func (e *requestError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// wrapRetryable 将传输错误包装为可重试错误
func wrapRetryable(err error) error {
	if err == nil {
		return nil
	}
	return &retryableError{err: err}
}
