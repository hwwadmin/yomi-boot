package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
)

// testServiceSeq 为每个测试客户端生成唯一服务名，便于日志区分不同测试的 client
var testServiceSeq atomic.Uint64

// newTestServiceName 返回测试用唯一服务名
func newTestServiceName() string {
	return fmt.Sprintf("test-svc-%d", testServiceSeq.Add(1))
}

// invokeTest 用默认超时构造 InvokeRequest 调用客户端
func invokeTest(c *HttpRpcClient, ctx context.Context, endpoint string, reqBody, respBody any) error {
	return c.Invoke(ctx, InvokeRequest{
		Endpoint: endpoint,
		ReqBody:  reqBody,
		RespBody: respBody,
	})
}

// resolverFunc 将函数适配为 EndpointResolver
type resolverFunc func(ctx context.Context, serviceName, pathSuffix string) (string, error)

// ResolveHTTPBase 实现 EndpointResolver
func (f resolverFunc) ResolveHTTPBase(ctx context.Context, serviceName, pathSuffix string) (string, error) {
	return f(ctx, serviceName, pathSuffix)
}

// invokeTestWith 指定最大尝试次数调用客户端
func invokeTestWith(c *HttpRpcClient, ctx context.Context, endpoint string, reqBody, respBody any, attempts int) error {
	return c.Invoke(ctx, InvokeRequest{
		Endpoint:    endpoint,
		ReqBody:     reqBody,
		RespBody:    respBody,
		MaxAttempts: attempts,
	})
}

// testEnvelope 测试用读响应信封
type testEnvelope struct {
	Code string          `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

// breakerSnapshot 读取熔断器状态与窗口长度
func breakerSnapshot(c *HttpRpcClient) (breakerState, int) {
	c.breaker.mu.Lock()
	defer c.breaker.mu.Unlock()
	return c.breaker.state, len(c.breaker.window)
}
