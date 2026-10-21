package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"

	"gitee.com/hq_project/yomi-boot/boot/errors"
)

// testServiceSeq 为每个测试客户端生成唯一服务名，避免进程级 breaker 共享串测
var testServiceSeq atomic.Uint64

// newTestServiceName 返回测试用唯一服务名
func newTestServiceName() string {
	return fmt.Sprintf("test-svc-%d", testServiceSeq.Add(1))
}

// testEnvelope 测试用读响应信封
type testEnvelope struct {
	Code  string          `json:"code"`
	Msg   string          `json:"msg"`
	Data  json.RawMessage `json:"data"`
	Total int64           `json:"total"`
}

// checkTestEnvelope 测试用业务判定 hook
func checkTestEnvelope(_ context.Context, call BizHookContext) (bool, error) {
	env, ok := call.RespBody.(*testEnvelope)
	if !ok {
		return false, errors.New("bad envelope type")
	}
	if env.Code == "" {
		return false, errors.Sentinel(ErrDownstream)
	}
	if env.Code != "OK" {
		return false, errors.NewBiz(env.Code, env.Msg)
	}
	return true, nil
}

// breakerSnapshot 读取熔断器状态与窗口长度
func breakerSnapshot(c *HttpRpcClient) (breakerState, int) {
	c.breaker.mu.Lock()
	defer c.breaker.mu.Unlock()
	return c.breaker.state, len(c.breaker.window)
}
