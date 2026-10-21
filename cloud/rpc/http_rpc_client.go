// @author zwei

package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/boot/appctx"
	"gitee.com/hq_project/yomi-boot/boot/errors"
	"gitee.com/hq_project/yomi-boot/boot/traceid"
)

// HttpRpcClient 基于 HTTP 的 RpcService 实现
// 只负责端点解析、发送请求、反序列化响应与传输级韧性
type HttpRpcClient struct {
	serviceName string
	resolver    EndpointResolver
	httpClient  *http.Client
	opts        httpClientOptions
	breaker     *circuitBreaker
}

// 编译期验证接口实现
var _ RpcService = (*HttpRpcClient)(nil)

// NewHttpRpcClient 创建绑定到指定服务名的 HTTP RPC 客户端
// resolver 负责端点解析；Consul 场景传 consul.NewRPCResolver(client)
// serviceName 为目标服务的注册名
// 熔断器按 serviceName 进程内共享；同一服务第一个 client 的 BreakerOptions 生效
func NewHttpRpcClient(resolver EndpointResolver, serviceName string, opts ...ClientOption) *HttpRpcClient {
	serviceName = strings.TrimSpace(serviceName)
	cfg := applyHTTPClientOptions(opts)
	return &HttpRpcClient{
		serviceName: serviceName,
		resolver:    resolver,
		httpClient:  &http.Client{Timeout: cfg.HTTPTimeout},
		opts:        cfg,
		breaker:     breakerForService(serviceName, cfg.Breaker),
	}
}

// Invoke 实现 RpcService.Invoke
// 传输成功 = HTTP 200 且响应反序列化成功；业务成功由 req.BizHook 判定
func (c *HttpRpcClient) Invoke(ctx context.Context, req InvokeRequest) error {
	if c == nil || c.httpClient == nil {
		return errors.New("HttpRpcClient 未初始化")
	}
	if req.BizHook == nil {
		return errors.New("rpc: BizHook 不能为空；纯传输请使用 rpc.NoopBizHook")
	}
	ctx = appctx.OrBackground(ctx)
	callerCtx := ctx
	if err := callerCtx.Err(); err != nil {
		return err
	}
	if c.serviceName == "" {
		return errors.New("rpc: 服务名不能为空")
	}

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	callCtx, cancel := context.WithTimeout(callerCtx, timeout)
	defer cancel()

	// 先过熔断再序列化：熔断打开时不浪费序列化；序列化失败释放半开探测位
	ticket, err := c.breaker.allow()
	if err != nil {
		return err
	}
	payload, err := json.Marshal(req.ReqBody)
	if err != nil {
		c.breaker.onNeutral(ticket)
		return errors.Wrap(err, "序列化请求体失败")
	}

	maxAttempts := req.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxAttempts
	}
	maxBytes := req.MaxResponseBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxResponseBytes
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := callCtx.Err(); err != nil {
			c.breaker.onNeutral(ticket)
			return err
		}

		url, err := c.resolveEndpoint(callCtx, req.Endpoint)
		if err != nil {
			if callerCtx.Err() != nil {
				c.breaker.onNeutral(ticket)
				return err
			}
			if !shouldRetry(callCtx, err, attempt, maxAttempts) {
				c.breaker.onNeutral(ticket)
				return err
			}
			if sleepErr := sleepBackoff(callCtx, backoffDuration(attempt, c.opts.BaseBackoff, c.opts.MaxBackoff)); sleepErr != nil {
				c.breaker.onNeutral(ticket)
				return sleepErr
			}
			continue
		}

		start := time.Now()
		body, err := c.doRequest(callCtx, url, payload, req.RespBody != nil, maxBytes)
		elapsed := time.Since(start)
		if err != nil {
			if callerCtx.Err() != nil {
				c.breaker.onNeutral(ticket)
				return err
			}
			if isLocalError(err) {
				c.breaker.onNeutral(ticket)
				return err
			}
			c.breaker.record(ticket, c.classify(err, elapsed))
			if !shouldRetry(callCtx, err, attempt, maxAttempts) {
				return err
			}
			if sleepErr := sleepBackoff(callCtx, backoffDuration(attempt, c.opts.BaseBackoff, c.opts.MaxBackoff)); sleepErr != nil {
				return sleepErr
			}
			continue
		}

		if req.RespBody != nil {
			if err := json.Unmarshal(body, req.RespBody); err != nil {
				protocolErr := &ProtocolError{Reason: "解析下游响应失败", Err: err}
				c.breaker.record(ticket, attemptResult{
					failure: true,
					slow:    elapsed > c.breaker.slowThreshold(),
					counted: true,
				})
				return protocolErr
			}
		}

		c.breaker.record(ticket, attemptResult{
			slow:    elapsed > c.breaker.slowThreshold(),
			counted: true,
		})
		ok, hookErr := req.BizHook(callCtx, BizHookContext{
			Endpoint: req.Endpoint,
			ReqBody:  req.ReqBody,
			RespBody: req.RespBody,
		})
		if hookErr != nil {
			return hookErr
		}
		if !ok {
			return errors.Sentinel(ErrBizRejected)
		}
		return nil
	}
	return errors.New("rpc: 未产生有效尝试")
}

// resolveEndpoint 通过 EndpointResolver 将 endpoint 解析为完整 HTTP URL
func (c *HttpRpcClient) resolveEndpoint(ctx context.Context, endpoint string) (string, error) {
	if c == nil {
		return "", errors.New("HttpRpcClient 未初始化")
	}
	if c.resolver == nil {
		return "", errors.New("HttpRpcClient 未初始化: EndpointResolver 为 nil")
	}
	url, err := c.resolver.ResolveHTTPBase(ctx, c.serviceName, endpoint)
	if err != nil {
		return "", &discoveryError{err: err}
	}
	return url, nil
}

// doRequest 发送一次 HTTP POST 并按需读取响应体
// needBody=false 时 200 和非 200 都直接丢弃 body；非 200 只用状态码、状态文本和 Retry-After
// 非 200 返回 HTTPStatusError；200 但读取或解析超限返回 ProtocolError
func (c *HttpRpcClient) doRequest(ctx context.Context, url string, payload []byte, needBody bool, maxBytes int64) ([]byte, error) {
	if strings.TrimSpace(url) == "" {
		return nil, &requestError{err: errors.New("请求地址不能为空")}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, &requestError{err: errors.Wrap(err, "创建请求失败")}
	}
	req.Header.Set("Content-Type", "application/json")
	traceid.SetTraceID(ctx, req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, wrapRetryable(errors.Wrap(err, "调用下游服务失败"))
	}
	defer func() { _ = resp.Body.Close() }()

	if !needBody {
		if resp.StatusCode == http.StatusOK {
			return nil, nil
		}
		return nil, &HTTPStatusError{
			Code:       resp.StatusCode,
			Detail:     http.StatusText(resp.StatusCode),
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
		}
	}

	if maxBytes <= 0 {
		maxBytes = defaultMaxResponseBytes
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, wrapRetryable(errors.Wrap(err, "读取下游响应失败"))
	}
	retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
	if int64(len(data)) > maxBytes {
		if resp.StatusCode == http.StatusOK {
			return nil, &ProtocolError{Reason: "响应体超过上限"}
		}
		return nil, &HTTPStatusError{Code: resp.StatusCode, Detail: "响应体超过上限", RetryAfter: retryAfter}
	}
	if resp.StatusCode != http.StatusOK {
		detail := strings.TrimSpace(string(data))
		if detail == "" {
			detail = http.StatusText(resp.StatusCode)
		} else if len(detail) > 200 {
			detail = detail[:200] + "..."
		}
		return nil, &HTTPStatusError{Code: resp.StatusCode, Detail: detail, RetryAfter: retryAfter}
	}
	return data, nil
}

// parseRetryAfter 解析 Retry-After 头，支持秒数与 HTTP 日期；最长 24 小时
func parseRetryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	const maxRetryAfter = 24 * time.Hour
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0
		}
		if seconds > int(maxRetryAfter/time.Second) {
			return maxRetryAfter
		}
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil {
		if d := time.Until(at); d > 0 {
			if d > maxRetryAfter {
				return maxRetryAfter
			}
			return d
		}
	}
	return 0
}

// classify 将传输错误归类为熔断结果
// 5xx/408 计失败并计入失败率样本；2xx 计入样本；4xx/429/3xx 不计失败率样本；
// 429 标记 throttled，半开探测遇到时重新打开；协议错误和网络错误计失败
func (c *HttpRpcClient) classify(err error, elapsed time.Duration) attemptResult {
	slow := elapsed > c.breaker.slowThreshold()
	var statusErr *HTTPStatusError
	if errors.As(err, &statusErr) {
		failure := statusErr.Code >= http.StatusInternalServerError || statusErr.Code == http.StatusRequestTimeout
		counted := failure || (statusErr.Code >= 200 && statusErr.Code < 300)
		return attemptResult{
			failure:   failure,
			slow:      slow,
			counted:   counted,
			throttled: statusErr.Code == http.StatusTooManyRequests,
		}
	}
	var protocolErr *ProtocolError
	if errors.As(err, &protocolErr) {
		return attemptResult{failure: true, slow: slow, counted: true}
	}
	return attemptResult{failure: true, counted: true}
}

// isLocalError 判断是否为本地请求构建错误
func isLocalError(err error) bool {
	var localErr *requestError
	return errors.As(err, &localErr)
}
