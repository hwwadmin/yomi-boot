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
// 负责端点解析、发送请求、反序列化响应、重试和熔断
type HttpRpcClient struct {
	serviceName string
	resolver    EndpointResolver
	httpClient  *http.Client
	opts        httpClientOptions
	breaker     *circuitBreaker
}

// 编译期验证接口实现
var _ RpcService = (*HttpRpcClient)(nil)

// NewHttpRpcClient 创建绑定服务名的 HTTP RPC 客户端
// resolver 负责端点解析；Consul 场景用 consul.NewConsulRPCResolver(client)
// 默认共享传输、不跟随重定向；WithTransport、WithCheckRedirect、WithLogBodies 可按需调整
// 不设置 client 级超时；单次 HTTP 尝试超时由 InvokeRequest.Timeout 控制
// resolver 为空或服务名为空时 panic，适合启动阶段装配
// 每个 client 独立熔断；同一服务建议复用一个 client
func NewHttpRpcClient(resolver EndpointResolver, serviceName string, opts ...ClientOption) *HttpRpcClient {
	if resolver == nil {
		panic(errors.New("rpc: EndpointResolver 不能为空"))
	}
	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" {
		panic(errors.New("rpc: 服务名不能为空"))
	}
	cfg := applyHTTPClientOptions(opts)
	return &HttpRpcClient{
		serviceName: serviceName,
		resolver:    resolver,
		httpClient: &http.Client{
			Transport:     cfg.Transport,
			CheckRedirect: cfg.CheckRedirect,
		},
		opts:    cfg,
		breaker: newCircuitBreaker(cfg.Breaker),
	}
}

// Invoke 实现 RpcService.Invoke
// 传输成功指 HTTP 200 且响应反序列化成功；业务成功由 req.BizHook 判定
// 每次尝试使用 req.Timeout；普通调用按 MaxAttempts 重试；半开探测只尝试一次
// 整个 Invoke 的总时长由调用方 ctx 控制，req.Timeout 不是总预算
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

	// 熔断通过后再序列化；序列化失败时释放探测位
	ticket, err := c.breaker.allow()
	if err != nil {
		c.logBreakerReject(ctx, req.Endpoint, 0)
		return err
	}
	// 正常路径由 record/onNeutral 结算探测位；异常退出时由这里释放
	defer func() { c.breaker.onNeutral(ticket) }()
	payload, err := json.Marshal(req.ReqBody)
	if err != nil {
		c.breaker.onNeutral(ticket)
		return &requestError{err: errors.Wrap(err, "序列化请求体失败")}
	}
	if c.opts.LogBodies {
		c.logRequest(ctx, req.Endpoint, payload)
	}

	maxAttempts := req.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxAttempts
	}
	maxBytes := req.MaxResponseBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxResponseBytes
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := callerCtx.Err(); err != nil {
			c.breaker.onNeutral(ticket)
			return err
		}
		if attempt > 1 {
			// 重试前重新检查熔断；半开时这里返回 ErrCircuitOpen，探测只发一个请求
			var allowErr error
			ticket, allowErr = c.breaker.allow()
			if allowErr != nil {
				c.logBreakerReject(ctx, req.Endpoint, attempt)
				if lastErr != nil {
					return lastErr
				}
				return allowErr
			}
		}
		// 探测票据只尝试一次
		probeOnly := ticket.probe
		if probeOnly {
			c.logBreakerProbe(ctx, req.Endpoint, attempt)
		}

		url, err := c.resolveEndpoint(callerCtx, req.Endpoint)
		if err != nil {
			lastErr = err
			if callerCtx.Err() != nil {
				c.breaker.onNeutral(ticket)
				return err
			}
			// 探测遇到发现错误时释放探测位，下一次调用重新探测
			if probeOnly || !shouldRetry(callerCtx, err, attempt, maxAttempts) {
				c.breaker.onNeutral(ticket)
				return err
			}
			delay := backoffDuration(attempt, c.opts.BaseBackoff, c.opts.MaxBackoff)
			if !canWait(callerCtx, delay) {
				c.breaker.onNeutral(ticket)
				c.logBudgetExhausted(ctx, req.Endpoint, attempt, maxAttempts, delay, err)
				return err
			}
			c.logRetry(ctx, req.Endpoint, attempt, maxAttempts, delay, err)
			if sleepErr := sleepBackoff(callerCtx, delay); sleepErr != nil {
				c.breaker.onNeutral(ticket)
				if callerCtx.Err() != nil {
					return sleepErr
				}
				c.logBudgetExhausted(ctx, req.Endpoint, attempt, maxAttempts, delay, err)
				return err
			}
			// 发现错误按中性处理，下一轮重新获取票据
			c.breaker.onNeutral(ticket)
			continue
		}

		start := time.Now()
		body, err := c.doRequestWithTimeout(callerCtx, timeout, url, payload, req.RespBody != nil, maxBytes)
		elapsed := time.Since(start)
		if c.opts.LogBodies {
			if err != nil {
				c.logResponse(ctx, req.Endpoint, attempt, statusCodeOf(err), body)
			} else if len(body) > 0 {
				c.logResponse(ctx, req.Endpoint, attempt, http.StatusOK, body)
			}
		}
		if err != nil {
			lastErr = err
			if callerCtx.Err() != nil {
				c.breaker.onNeutral(ticket)
				return err
			}
			if isLocalError(err) {
				c.breaker.onNeutral(ticket)
				return err
			}
			// 熔断打开后结束本次调用，返回当前错误
			if c.breaker.record(ticket, c.classify(err, elapsed)) {
				c.logBreakerOpen(ctx, req.Endpoint, attempt, reasonOf(err), err)
				return err
			}
			// 探测只尝试一次
			if probeOnly {
				return err
			}
			if !shouldRetry(callerCtx, err, attempt, maxAttempts) {
				return err
			}
			delay := backoffDuration(attempt, c.opts.BaseBackoff, c.opts.MaxBackoff)
			var statusErr *HTTPStatusError
			if errors.As(err, &statusErr) && statusErr.RetryAfter > delay {
				delay = statusErr.RetryAfter
			}
			// 剩余预算不够等待时直接返回下游错误
			if !canWait(callerCtx, delay) {
				c.logBudgetExhausted(ctx, req.Endpoint, attempt, maxAttempts, delay, err)
				return err
			}
			c.logRetry(ctx, req.Endpoint, attempt, maxAttempts, delay, err)
			if sleepErr := sleepBackoff(callerCtx, delay); sleepErr != nil {
				if callerCtx.Err() != nil {
					return sleepErr
				}
				c.logBudgetExhausted(ctx, req.Endpoint, attempt, maxAttempts, delay, err)
				return err
			}
			continue
		}

		if req.RespBody != nil {
			if err := json.Unmarshal(body, req.RespBody); err != nil {
				protocolErr := &ProtocolError{Reason: "解析下游响应失败", Err: err}
				if c.breaker.record(ticket, attemptResult{
					failure: true,
					slow:    elapsed > c.breaker.slowThreshold(),
					counted: true,
				}) {
					c.logBreakerOpen(ctx, req.Endpoint, attempt, "protocol", protocolErr)
				}
				return protocolErr
			}
		}

		// 成功响应照常返回；仅当本次结果打开熔断时记录 breaker.open
		slow := elapsed > c.breaker.slowThreshold()
		switch c.breaker.recordOutcome(ticket, attemptResult{slow: slow, counted: true}) {
		case breakerRecordOpenedByFailure:
			c.logBreakerOpen(ctx, req.Endpoint, attempt, "failure_rate", nil)
		case breakerRecordOpenedBySlow:
			c.logBreakerOpen(ctx, req.Endpoint, attempt, "slow", nil)
		case breakerRecordOpenedByProbe:
			reason := "probe"
			if slow {
				reason = "slow"
			}
			c.logBreakerOpen(ctx, req.Endpoint, attempt, reason, nil)
		}
		ok, hookErr := req.BizHook(callerCtx, BizHookContext{
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
		if attempt > 1 {
			c.logRetryRecovered(ctx, req.Endpoint, attempt)
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return errors.New("rpc: 未产生有效尝试")
}

// doRequestWithTimeout 为单次 HTTP 尝试派生独立超时上下文
func (c *HttpRpcClient) doRequestWithTimeout(ctx context.Context, timeout time.Duration, url string, payload []byte, needBody bool, maxBytes int64) ([]byte, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return c.doRequest(attemptCtx, url, payload, needBody, maxBytes)
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

// maxDrainBytes 是 void 调用排空响应体的上限，便于复用连接
const maxDrainBytes = 4 << 10

// doRequest 发送一次 HTTP POST 并按需读取响应体
// needBody=false 时排空响应体；非 200 只返回状态码、状态文本和 Retry-After
// needBody=true 时非 200 也读取 body；Detail 取前 200 字节，body 供日志使用
// 非 200 返回 HTTPStatusError；200 读取或解析超限返回 ProtocolError
// 出错时返回已读 body，供调用方记录日志
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
		// CheckRedirect 拒绝时 Client 会返回响应和 *url.Error；按本地 requestError 处理
		if resp != nil {
			return nil, &requestError{err: errors.Wrap(err, "重定向策略拒绝")}
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, wrapRetryable(errors.Wrap(err, "调用下游服务失败"))
	}
	defer func() { _ = resp.Body.Close() }()

	if !needBody {
		drainBody(resp.Body)
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
			return data, &ProtocolError{Reason: "响应体超过上限"}
		}
		return data, &HTTPStatusError{Code: resp.StatusCode, Detail: "响应体超过上限", RetryAfter: retryAfter}
	}
	if resp.StatusCode != http.StatusOK {
		detail := strings.TrimSpace(string(data))
		if detail == "" {
			detail = http.StatusText(resp.StatusCode)
		} else if len(detail) > 200 {
			detail = detail[:200] + "..."
		}
		return data, &HTTPStatusError{Code: resp.StatusCode, Detail: detail, RetryAfter: retryAfter}
	}
	return data, nil
}

// drainBody 最多排空 maxDrainBytes+1 字节，随后由调用方关闭响应体
func drainBody(body io.Reader) {
	if body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(body, maxDrainBytes+1))
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

// classify 按状态码和耗时归类熔断结果
// 5xx/408、协议错误、网络错误计失败；2xx 计成功样本；4xx/429/3xx 不进失败率样本
// 429 另记 throttled，半开探测遇到时重新打开
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
