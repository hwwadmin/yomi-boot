// Package rpc 基于 HTTP 协议的 RPC 服务调用实现，通过 Consul 服务发现完成端点解析与远程调用
//
// 面向同步读路径：轻量重试（退避）+ 按下游服务熔断。跨域写走 DTM，不在此叠加重试语义。
// 响应信封为微服务间固定契约（见微服务调用规范），与对外 Encoder 无关。
package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"gitee.com/hq_project/yomi-boot/consul"
	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/traceid"
	"io"
	"net/http"
	"strings"
)

// ErrDownstream 下游服务异常
var ErrDownstream = errors.NewNoStack("DOWNSTREAM_ERROR")

// circuitOpenError 熔断打开；Error 为 CIRCUIT_OPEN，Unwrap 到 ErrDownstream 以支持 errors.Is
type circuitOpenError struct{}

// Error 返回熔断打开错误码
func (circuitOpenError) Error() string { return "CIRCUIT_OPEN" }

// Unwrap 返回下游异常哨兵，便于 errors.Is 匹配 ErrDownstream
func (circuitOpenError) Unwrap() error { return ErrDownstream }

// ErrCircuitOpen 下游熔断已打开（errors.Is 仍匹配 ErrDownstream）
var ErrCircuitOpen error = circuitOpenError{}

// rpcEnvelope 微服务间读响应信封；Data 用 RawMessage 延迟解码，避免 int64 精度丢失。
type rpcEnvelope struct {
	Code  string          `json:"code"`
	Msg   string          `json:"msg"`
	Data  json.RawMessage `json:"data"`
	Total int64           `json:"total"`
}

// HttpRpcClient 基于 HTTP 协议的 RpcService 实现
// 内部组合 Consul 服务发现与标准 HTTP 客户端，完成端点解析、请求发送与响应反序列化
type HttpRpcClient struct {
	serviceName string
	consul      *consul.Client
	httpClient  *http.Client
	opts        ClientOptions
	breaker     *circuitBreaker
	// testResolve 仅测试注入：非 nil 时替代 Consul 解析
	testResolve func(ctx context.Context, endpoint string) (string, error)
}

// 编译期验证接口实现
var _ RpcService = (*HttpRpcClient)(nil)

// NewHttpRpcClient 创建绑定到指定服务名的 HTTP RPC 客户端
// consulClient 用于服务发现，serviceName 为目标微服务的 Consul 注册名
func NewHttpRpcClient(consulClient *consul.Client, serviceName string, opts ...ClientOption) *HttpRpcClient {
	cfg := applyClientOptions(opts)
	return &HttpRpcClient{
		serviceName: serviceName,
		consul:      consulClient,
		httpClient:  &http.Client{Timeout: cfg.HTTPTimeout},
		opts:        cfg,
		breaker:     newCircuitBreaker(cfg.BreakerFailures, cfg.BreakerOpenDuration),
	}
}

// resolveEndpoint 通过 Consul 服务发现将 endpoint 解析为完整 HTTP URL
func (c *HttpRpcClient) resolveEndpoint(ctx context.Context, endpoint string) (string, error) {
	if c == nil {
		return "", errors.New("HttpRpcClient 未初始化")
	}
	if c.testResolve != nil {
		url, err := c.testResolve(ctx, endpoint)
		if err != nil {
			return "", wrapRetryable(err)
		}
		return url, nil
	}
	if c.consul == nil {
		return "", errors.New("HttpRpcClient 未初始化")
	}
	url, err := c.consul.ResolveHTTPBase(ctx, c.serviceName, endpoint)
	if err != nil {
		return "", wrapRetryable(err)
	}
	return url, nil
}

// Invoke 实现 RpcService.Invoke，发送 HTTP POST 请求并反序列化响应到 respData
func (c *HttpRpcClient) Invoke(ctx context.Context, endpoint string, reqBody any, respData any) error {
	_, err := c.invokeWithRetry(ctx, endpoint, reqBody, respData)
	return err
}

// InvokePage 实现 RpcService.InvokePage，发送 HTTP POST 分页请求并返回总条数
func (c *HttpRpcClient) InvokePage(ctx context.Context, endpoint string, reqBody any, respData any) (int64, error) {
	return c.invokeWithRetry(ctx, endpoint, reqBody, respData)
}

// invokeWithRetry 读路径：熔断检查 → 解析地址 → 调用；传输失败短退避重试并重新发现
func (c *HttpRpcClient) invokeWithRetry(ctx context.Context, endpoint string, reqBody any, respData any) (int64, error) {
	if c == nil {
		return 0, errors.New("HttpRpcClient 未初始化")
	}
	if err := c.breaker.allow(); err != nil {
		return 0, err
	}

	maxAttempts := c.opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxAttempts
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			c.recordAttemptResult(lastErr)
			return 0, err
		}

		url, err := c.resolveEndpoint(ctx, endpoint)
		if err != nil {
			lastErr = err
			if !c.shouldRetry(ctx, err, attempt, maxAttempts) {
				c.recordAttemptResult(err)
				return 0, err
			}
			if sleepErr := sleepBackoff(ctx, backoffDuration(attempt, c.opts.BaseBackoff, c.opts.MaxBackoff)); sleepErr != nil {
				c.recordAttemptResult(err)
				return 0, sleepErr
			}
			continue
		}

		total, err := c.doPostInternal(ctx, url, reqBody, respData)
		if err == nil {
			c.breaker.onSuccess()
			return total, nil
		}
		lastErr = err
		if isTransportSuccess(err) {
			c.breaker.onSuccess()
			return 0, err
		}
		if !c.shouldRetry(ctx, err, attempt, maxAttempts) {
			c.recordAttemptResult(err)
			return 0, err
		}
		if sleepErr := sleepBackoff(ctx, backoffDuration(attempt, c.opts.BaseBackoff, c.opts.MaxBackoff)); sleepErr != nil {
			c.recordAttemptResult(err)
			return 0, sleepErr
		}
	}
	c.recordAttemptResult(lastErr)
	return 0, lastErr
}

// shouldRetry 判断是否继续重试：未超次数、上下文未取消且错误可重试
func (c *HttpRpcClient) shouldRetry(ctx context.Context, err error, attempt, maxAttempts int) bool {
	if attempt >= maxAttempts {
		return false
	}
	if ctx.Err() != nil {
		return false
	}
	return isRetryableRPCError(err)
}

// recordAttemptResult 按本次结果更新熔断：传输成功/非重试失败记成功，可重试失败记失败
func (c *HttpRpcClient) recordAttemptResult(err error) {
	if isTransportSuccess(err) {
		c.breaker.onSuccess()
		return
	}
	if isRetryableRPCError(err) {
		c.breaker.onFailure()
		return
	}
	// 非传输失败（如 4xx、解析错误）：服务通常仍存活，不累计熔断
	c.breaker.onSuccess()
}

// doRequest 执行 HTTP POST 请求的公共流程：前置校验、序列化、发送请求、读取响应体
func (c *HttpRpcClient) doRequest(ctx context.Context, url string, reqBody any) ([]byte, error) {
	// 前置校验：客户端可用性
	if c == nil || c.httpClient == nil {
		return nil, errors.New("HttpRpcClient 未初始化")
	}
	// 前置校验：请求地址非空
	if strings.TrimSpace(url) == "" {
		return nil, errors.New("请求地址不能为空")
	}

	// 序列化请求体
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, errors.Wrap(err, "序列化请求体失败")
	}

	// 构建 HTTP 请求
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, errors.Wrap(err, "创建请求失败")
	}
	req.Header.Set("Content-Type", "application/json")
	traceid.SetTraceID(ctx, req)

	// 发送 HTTP 请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, errors.Wrap(err, "调用下游服务失败")
		}
		return nil, wrapRetryable(errors.Wrap(err, "调用下游服务失败"))
	}
	defer func() { _ = resp.Body.Close() }()

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, wrapRetryable(errors.Wrap(err, "读取下游响应失败"))
	}
	if resp.StatusCode >= http.StatusBadRequest {
		detail := strings.TrimSpace(string(body))
		if detail == "" {
			detail = http.StatusText(resp.StatusCode)
		} else if len(detail) > 200 {
			detail = detail[:200] + "..."
		}
		return nil, &httpStatusError{code: resp.StatusCode, detail: detail}
	}
	return body, nil
}

// doPostInternal 执行一次 HTTP POST 调用，按微服务间信封解析并填充 respData；分页响应时返回 total
func (c *HttpRpcClient) doPostInternal(ctx context.Context, url string, reqBody any, respData any) (int64, error) {
	body, err := c.doRequest(ctx, url, reqBody)
	if err != nil {
		return 0, err
	}

	var envelope rpcEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return 0, errors.Wrap(err, "解析下游响应失败")
	}

	if envelope.Code != "OK" {
		if envelope.Code != "" {
			return 0, errors.NewBiz(envelope.Code, envelope.Msg)
		}
		return 0, errors.Sentinel(ErrDownstream)
	}

	if respData != nil && len(envelope.Data) > 0 {
		trimmed := bytes.TrimSpace(envelope.Data)
		if len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
			if err := json.Unmarshal(envelope.Data, respData); err != nil {
				return 0, errors.Wrap(err, "解析下游 data 失败")
			}
		}
	}
	return envelope.Total, nil
}
