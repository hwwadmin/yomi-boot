// @author zwei

package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/boot/appctx"
	"gitee.com/hq_project/yomi-boot/boot/errors"
	"gitee.com/hq_project/yomi-boot/boot/traceid"
	"gitee.com/hq_project/yomi-boot/cloud/consul"
)

// HttpRpcClient 基于 HTTP 的 RpcService 实现
// 只负责服务发现、发送请求、反序列化响应与传输级韧性
type HttpRpcClient struct {
	serviceName string
	consul      *consul.Client
	httpClient  *http.Client
	opts        clientOptions
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
		breaker:     newCircuitBreaker(cfg.Breaker),
	}
}

// Invoke 实现 RpcService.Invoke
// 传输成功 = HTTP 200 且响应反序列化成功；业务成功由 bizHook 判定
func (c *HttpRpcClient) Invoke(ctx context.Context, endpoint string, reqBody any, respBody any, bizHook BizHookFunc) error {
	if c == nil || c.httpClient == nil {
		return errors.New("HttpRpcClient 未初始化")
	}
	ctx = appctx.OrBackground(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.testResolve == nil && strings.TrimSpace(c.serviceName) == "" {
		return errors.New("rpc: 服务名不能为空")
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return errors.Wrap(err, "序列化请求体失败")
	}
	ticket, err := c.breaker.allow()
	if err != nil {
		return err
	}

	maxAttempts := c.opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxAttempts
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			c.breaker.onNeutral(ticket)
			return err
		}

		url, err := c.resolveEndpoint(ctx, endpoint)
		if err != nil {
			if ctx.Err() != nil {
				c.breaker.onNeutral(ticket)
				return err
			}
			if !shouldRetry(ctx, err, attempt, maxAttempts) {
				c.breaker.onNeutral(ticket)
				return err
			}
			if sleepErr := sleepBackoff(ctx, backoffDuration(attempt, c.opts.BaseBackoff, c.opts.MaxBackoff)); sleepErr != nil {
				c.breaker.onNeutral(ticket)
				return sleepErr
			}
			continue
		}

		start := time.Now()
		body, err := c.doRequest(ctx, url, payload)
		elapsed := time.Since(start)
		if err != nil {
			if ctx.Err() != nil {
				c.breaker.onNeutral(ticket)
				return err
			}
			if isLocalError(err) {
				c.breaker.onNeutral(ticket)
				return err
			}
			c.breaker.record(ticket, c.classify(err, elapsed))
			if !shouldRetry(ctx, err, attempt, maxAttempts) {
				return err
			}
			if sleepErr := sleepBackoff(ctx, backoffDuration(attempt, c.opts.BaseBackoff, c.opts.MaxBackoff)); sleepErr != nil {
				return sleepErr
			}
			continue
		}

		if respBody != nil {
			if err := json.Unmarshal(body, respBody); err != nil {
				protocolErr := &ProtocolError{Reason: "解析下游响应失败", Err: err}
				c.breaker.record(ticket, attemptResult{failure: true, slow: elapsed > c.breaker.slowThreshold()})
				return protocolErr
			}
		}

		c.breaker.record(ticket, attemptResult{slow: elapsed > c.breaker.slowThreshold()})
		if bizHook != nil {
			if err := bizHook(ctx, BizHookContext{
				Endpoint: endpoint,
				ReqBody:  reqBody,
				RespBody: respBody,
			}); err != nil {
				return err
			}
		}
		return nil
	}
	return errors.New("rpc: 未产生有效尝试")
}

// resolveEndpoint 通过 Consul 服务发现将 endpoint 解析为完整 HTTP URL
func (c *HttpRpcClient) resolveEndpoint(ctx context.Context, endpoint string) (string, error) {
	if c == nil {
		return "", errors.New("HttpRpcClient 未初始化")
	}
	if c.testResolve != nil {
		url, err := c.testResolve(ctx, endpoint)
		if err != nil {
			return "", &discoveryError{err: err}
		}
		return url, nil
	}
	if c.consul == nil {
		return "", errors.New("HttpRpcClient 未初始化: consul 客户端为 nil")
	}
	url, err := c.consul.ResolveHTTPBase(ctx, c.serviceName, endpoint)
	if err != nil {
		return "", &discoveryError{err: err}
	}
	return url, nil
}

// doRequest 发送一次 HTTP POST 并读取响应体
// 非 200 返回 HTTPStatusError；200 但读取或解析超限返回 ProtocolError
func (c *HttpRpcClient) doRequest(ctx context.Context, url string, payload []byte) ([]byte, error) {
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

	maxBytes := c.opts.MaxResponseBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxResponseBytes
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, wrapRetryable(errors.Wrap(err, "读取下游响应失败"))
	}
	if int64(len(data)) > maxBytes {
		if resp.StatusCode == http.StatusOK {
			return nil, &ProtocolError{Reason: "响应体超过上限"}
		}
		return nil, &HTTPStatusError{Code: resp.StatusCode, Detail: "响应体超过上限"}
	}
	if resp.StatusCode != http.StatusOK {
		detail := strings.TrimSpace(string(data))
		if detail == "" {
			detail = http.StatusText(resp.StatusCode)
		} else if len(detail) > 200 {
			detail = detail[:200] + "..."
		}
		return nil, &HTTPStatusError{Code: resp.StatusCode, Detail: detail}
	}
	return data, nil
}

// classify 将传输错误归类为熔断结果
// 5xx/408 计失败；4xx/429/3xx 不计失败；协议错误和网络错误计失败
func (c *HttpRpcClient) classify(err error, elapsed time.Duration) attemptResult {
	slow := elapsed > c.breaker.slowThreshold()
	var statusErr *HTTPStatusError
	if errors.As(err, &statusErr) {
		return attemptResult{
			failure: statusErr.Code >= http.StatusInternalServerError || statusErr.Code == http.StatusRequestTimeout,
			slow:    slow,
		}
	}
	var protocolErr *ProtocolError
	if errors.As(err, &protocolErr) {
		return attemptResult{failure: true, slow: slow}
	}
	return attemptResult{failure: true}
}

// isLocalError 判断是否为本地请求构建错误
func isLocalError(err error) bool {
	var localErr *requestError
	return errors.As(err, &localErr)
}
