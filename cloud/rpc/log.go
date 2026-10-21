// @author zwei

package rpc

import (
	"context"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/boot/errors"
	"gitee.com/hq_project/yomi-boot/boot/logger"
)

// defaultMaxLogBodyBytes 是请求/响应体日志的截断上限
const defaultMaxLogBodyBytes = 4 << 10

// logRequest 记录请求体；仅在 LogBodies 开启时调用
func (c *HttpRpcClient) logRequest(ctx context.Context, endpoint string, payload []byte) {
	logger.Debug(ctx, "rpc.request",
		"service", c.serviceName,
		"endpoint", endpoint,
		"body", truncateLogBody(payload),
	)
}

// logResponse 记录响应体；仅在 LogBodies 开启时调用
func (c *HttpRpcClient) logResponse(ctx context.Context, endpoint string, attempt, status int, body []byte) {
	fields := []any{
		"service", c.serviceName,
		"endpoint", endpoint,
		"attempt", attempt,
	}
	if status > 0 {
		fields = append(fields, "status", status)
	}
	fields = append(fields, "body", truncateLogBody(body))
	logger.Debug(ctx, "rpc.response", fields...)
}

// truncateLogBody 截断到 4 KiB，保证 UTF-8 有效，并标记截断
func truncateLogBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	truncated := false
	if len(body) > defaultMaxLogBodyBytes {
		body = body[:defaultMaxLogBodyBytes]
		truncated = true
	}
	text := strings.ToValidUTF8(string(body), "\uFFFD")
	if truncated {
		text += "...(truncated)"
	}
	return text
}

// statusCodeOf 返回 HTTP 状态码；非 HTTP 错误返回 0
func statusCodeOf(err error) int {
	var statusErr *HTTPStatusError
	if errors.As(err, &statusErr) {
		return statusErr.Code
	}
	return 0
}

// logRetry 记录一次即将执行的重试，不输出请求/响应体
func (c *HttpRpcClient) logRetry(ctx context.Context, endpoint string, attempt, maxAttempts int, delay time.Duration, err error) {
	fields := []any{
		"service", c.serviceName,
		"endpoint", endpoint,
		"attempt", attempt,
		"max_attempts", maxAttempts,
		"delay", delay.String(),
		"reason", reasonOf(err),
	}
	fields = appendStatusFields(fields, err)
	logger.Warn(ctx, "rpc.retry", fields...)
}

// logBudgetExhausted 记录因本次调用预算不足而放弃重试
func (c *HttpRpcClient) logBudgetExhausted(ctx context.Context, endpoint string, attempt, maxAttempts int, delay time.Duration, err error) {
	fields := []any{
		"service", c.serviceName,
		"endpoint", endpoint,
		"attempt", attempt,
		"max_attempts", maxAttempts,
		"delay", delay.String(),
		"reason", reasonOf(err),
	}
	fields = appendStatusFields(fields, err)
	logger.Warn(ctx, "rpc.retry.budget_exhausted", fields...)
}

// logBreakerOpen 记录熔断打开；err 只用于补充状态码/Retry-After，不输出响应体
func (c *HttpRpcClient) logBreakerOpen(ctx context.Context, endpoint string, attempt int, reason string, err error) {
	fields := []any{
		"service", c.serviceName,
		"endpoint", endpoint,
		"attempt", attempt,
		"reason", reason,
	}
	fields = appendStatusFields(fields, err)
	logger.Warn(ctx, "rpc.breaker.open", fields...)
}

// logBreakerReject 记录因熔断打开而拒绝的调用
func (c *HttpRpcClient) logBreakerReject(ctx context.Context, endpoint string, attempt int) {
	logger.Debug(ctx, "rpc.breaker.reject",
		"service", c.serviceName,
		"endpoint", endpoint,
		"attempt", attempt,
	)
}

// logBreakerProbe 记录一次半开探测
func (c *HttpRpcClient) logBreakerProbe(ctx context.Context, endpoint string, attempt int) {
	logger.Info(ctx, "rpc.breaker.probe",
		"service", c.serviceName,
		"endpoint", endpoint,
		"attempt", attempt,
	)
}

// logRetryRecovered 记录重试后成功
func (c *HttpRpcClient) logRetryRecovered(ctx context.Context, endpoint string, attempt int) {
	logger.Info(ctx, "rpc.retry.recovered",
		"service", c.serviceName,
		"endpoint", endpoint,
		"attempt", attempt,
	)
}

// appendStatusFields 追加 HTTP 状态码与 Retry-After；不输出下游响应体
func appendStatusFields(fields []any, err error) []any {
	var statusErr *HTTPStatusError
	if errors.As(err, &statusErr) {
		fields = append(fields, "status", statusErr.Code)
		if statusErr.RetryAfter > 0 {
			fields = append(fields, "retry_after", statusErr.RetryAfter.String())
		}
	}
	return fields
}

// reasonOf 返回错误的简短类别，用于日志；不包含下游响应体
func reasonOf(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline"
	}
	var statusErr *HTTPStatusError
	if errors.As(err, &statusErr) {
		return "http"
	}
	var protocolErr *ProtocolError
	if errors.As(err, &protocolErr) {
		return "protocol"
	}
	var discoveryErr *discoveryError
	if errors.As(err, &discoveryErr) {
		return "discovery"
	}
	var retryErr *retryableError
	if errors.As(err, &retryErr) {
		return "transport"
	}
	return "error"
}
