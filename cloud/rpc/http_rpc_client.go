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
// resolver 负责端点解析；Consul 场景传 consul.NewConsulRPCResolver(client)
// serviceName 为目标服务的注册名
// 默认使用共享传输（每 host 最多 100 个空闲连接），默认不跟随重定向；
// 需要 mTLS/私有 CA/代理/连接池时用 WithTransport，需要跟随重定向时用 WithCheckRedirect
// 需要打印请求/响应体时用 WithLogBodies（建议仅 dev 开启，默认关闭）
// resolver 为空或 serviceName 为空时 panic，用于启动装配阶段 Fail Fast
// 每个 client 使用独立熔断器；同一服务建议复用一个 client
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
			Timeout:       cfg.HTTPTimeout,
			Transport:     cfg.Transport,
			CheckRedirect: cfg.CheckRedirect,
		},
		opts:    cfg,
		breaker: newCircuitBreaker(cfg.Breaker),
	}
}

// Invoke 实现 RpcService.Invoke
// 传输成功 = HTTP 200 且响应反序列化成功；业务成功由 req.BizHook 判定
// 半开状态下只放行一个探测请求，探测不做内部重试
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
		c.logBreakerReject(ctx, req.Endpoint, 0)
		return err
	}
	// 兜底释放探测位：正常路径由 record/onNeutral 结算；panic 或未来新增的提前返回
	// 也要把半开探测位放掉，避免熔断永久卡在半开
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
		if err := callCtx.Err(); err != nil {
			c.breaker.onNeutral(ticket)
			return err
		}
		if attempt > 1 {
			// 每次重试重新过熔断：上一轮失败可能已把熔断打开；
			// 半开探测失败后这里会拿到 ErrCircuitOpen，保证一次探测只发一个请求
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
		// 半开探测只放行一个请求：本次票据是探测票据时，不做内部重试
		probeOnly := ticket.probe
		if probeOnly {
			c.logBreakerProbe(ctx, req.Endpoint, attempt)
		}

		url, err := c.resolveEndpoint(callCtx, req.Endpoint)
		if err != nil {
			lastErr = err
			if callerCtx.Err() != nil {
				c.breaker.onNeutral(ticket)
				return err
			}
			// 探测不做发现重试：中性结果只释放探测位，下一次调用重新探测
			if probeOnly || !shouldRetry(callCtx, err, attempt, maxAttempts) {
				c.breaker.onNeutral(ticket)
				return err
			}
			delay := backoffDuration(attempt, c.opts.BaseBackoff, c.opts.MaxBackoff)
			if !canWait(callCtx, delay) {
				c.breaker.onNeutral(ticket)
				c.logBudgetExhausted(ctx, req.Endpoint, attempt, maxAttempts, delay, err)
				return err
			}
			c.logRetry(ctx, req.Endpoint, attempt, maxAttempts, delay, err)
			if sleepErr := sleepBackoff(callCtx, delay); sleepErr != nil {
				c.breaker.onNeutral(ticket)
				if callerCtx.Err() != nil {
					return sleepErr
				}
				c.logBudgetExhausted(ctx, req.Endpoint, attempt, maxAttempts, delay, err)
				return err
			}
			// 发现失败不反映下游健康度；释放探测位后下一轮重新 allow
			c.breaker.onNeutral(ticket)
			continue
		}

		start := time.Now()
		body, err := c.doRequest(callCtx, url, payload, req.RespBody != nil, maxBytes)
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
			// 熔断打开后立即停止本次调用的剩余重试，避免继续打已判定故障的下游
			if c.breaker.record(ticket, c.classify(err, elapsed)) {
				c.logBreakerOpen(ctx, req.Endpoint, attempt, reasonOf(err), err)
				return err
			}
			// 半开探测只发一个请求，不进入重试
			if probeOnly {
				return err
			}
			if !shouldRetry(callCtx, err, attempt, maxAttempts) {
				return err
			}
			delay := backoffDuration(attempt, c.opts.BaseBackoff, c.opts.MaxBackoff)
			var statusErr *HTTPStatusError
			if errors.As(err, &statusErr) && statusErr.RetryAfter > delay {
				delay = statusErr.RetryAfter
			}
			// 等待放不进剩余预算时直接返回本次下游错误，别换成 context deadline exceeded
			if !canWait(callCtx, delay) {
				c.logBudgetExhausted(ctx, req.Endpoint, attempt, maxAttempts, delay, err)
				return err
			}
			c.logRetry(ctx, req.Endpoint, attempt, maxAttempts, delay, err)
			if sleepErr := sleepBackoff(callCtx, delay); sleepErr != nil {
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

		// 成功响应照常返回；只有本次结果真正打开了熔断才记 breaker.open，
		// 避免熔断已被其他调用打开时把快速成功误标成 slow
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
		if attempt > 1 {
			c.logRetryRecovered(ctx, req.Endpoint, attempt)
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

// maxDrainBytes 是 void 调用排空响应体的上限；小响应可复用连接，大响应直接关闭
const maxDrainBytes = 4 << 10

// doRequest 发送一次 HTTP POST 并按需读取响应体
// needBody=false 时最多排空 maxDrainBytes 后丢弃 body；此时非 200 只用状态码、状态文本和 Retry-After
// needBody=true 时非 200 会读取并返回已读 body；Detail 取前 200 字节，body 供调用方记录日志
// 非 200 返回 HTTPStatusError；200 但读取或解析超限返回 ProtocolError
// 出错时若已读到 body 会一并返回，仅供调用方记录日志，不得当成功响应使用
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
		// CheckRedirect 返回错误时，Go 会同时返回上一次响应（Body 已由 Client 关闭）
		// 和 *url.Error；这是本地重定向策略拒绝，不是下游故障：不重试、不计熔断。
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
