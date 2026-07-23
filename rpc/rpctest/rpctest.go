// Package rpctest 提供 RpcService 接口的测试桩，方便单元测试时注入模拟实现
//
// @author zwei
package rpctest

import (
	"context"

	"gitee.com/hq_project/yomi-boot/rpc"
)

// TestRpcService 可配置的 RpcService 测试桩
// 通过设置 InvokeFunc / InvokePageFunc 函数字段控制每个方法的返回值或副作用
// 零值实例的 Invoke 返回 nil、InvokePage 返回 (0, nil)，可安全使用
type TestRpcService struct {
	// InvokeFunc 自定义 Invoke 行为，为 nil 时返回 nil
	InvokeFunc func(ctx context.Context, endpoint string, reqBody any, respData any) error
	// InvokePageFunc 自定义 InvokePage 行为，为 nil 时返回 (0, nil)
	InvokePageFunc func(ctx context.Context, endpoint string, reqBody any, respData any) (int64, error)
}

// 编译期验证接口实现
var _ rpc.RpcService = (*TestRpcService)(nil)

// NewTestRpcService 创建 TestRpcService 实例
func NewTestRpcService() *TestRpcService {
	return &TestRpcService{}
}

// Invoke 实现 RpcService.Invoke
func (m *TestRpcService) Invoke(ctx context.Context, endpoint string, reqBody any, respData any) error {
	if m.InvokeFunc != nil {
		return m.InvokeFunc(ctx, endpoint, reqBody, respData)
	}
	return nil
}

// InvokePage 实现 RpcService.InvokePage
func (m *TestRpcService) InvokePage(ctx context.Context, endpoint string, reqBody any, respData any) (int64, error) {
	if m.InvokePageFunc != nil {
		return m.InvokePageFunc(ctx, endpoint, reqBody, respData)
	}
	return 0, nil
}
