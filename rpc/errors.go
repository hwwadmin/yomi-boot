// Package rpc 定义 RPC 传输层错误类型
//
// @author zwei
package rpc

import (
	"fmt"
	"net/http"
)

// retryableError 标记可重试的传输错误（网络、服务发现等）
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

// wrapRetryable 将传输错误包装为可重试错误
func wrapRetryable(err error) error {
	if err == nil {
		return nil
	}
	return &retryableError{err: err}
}

// httpStatusError 下游 HTTP 状态错误
type httpStatusError struct {
	code   int
	detail string
}

// Error 返回 HTTP 状态错误描述
func (e *httpStatusError) Error() string {
	if e == nil {
		return "rpc: http status error"
	}
	return fmt.Sprintf("下游 HTTP %d: %s", e.code, e.detail)
}

// retryable 判断 HTTP 状态码是否可重试
func (e *httpStatusError) retryable() bool {
	if e == nil {
		return false
	}
	switch e.code {
	case http.StatusRequestTimeout, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}
