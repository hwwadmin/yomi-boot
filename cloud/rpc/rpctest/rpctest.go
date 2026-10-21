// @author zwei

package rpctest

import (
	"context"

	"gitee.com/hq_project/yomi-boot/cloud/rpc"
)

// TestRpcService 可配置的 RpcService 测试桩
// 零值实例的 Invoke 返回 nil，可安全使用
type TestRpcService struct {
	// InvokeFunc 自定义 Invoke 行为，为 nil 时返回 nil
	InvokeFunc func(ctx context.Context, endpoint string, reqBody any, respBody any, bizHook rpc.BizHookFunc) error
}

// 编译期验证接口实现
var _ rpc.RpcService = (*TestRpcService)(nil)

// NewTestRpcService 创建 TestRpcService 实例
func NewTestRpcService() *TestRpcService {
	return &TestRpcService{}
}

// Invoke 实现 RpcService.Invoke
func (m *TestRpcService) Invoke(ctx context.Context, endpoint string, reqBody any, respBody any, bizHook rpc.BizHookFunc) error {
	if m.InvokeFunc != nil {
		return m.InvokeFunc(ctx, endpoint, reqBody, respBody, bizHook)
	}
	return nil
}
