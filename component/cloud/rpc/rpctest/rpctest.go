// @author zwei

package rpctest

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/cloud/rpc"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// TestRpcService 可配置的 RpcService 测试桩
// BizHook 必填；InvokeFunc 留空时 Invoke 返回 nil
type TestRpcService struct {
	// InvokeFunc 自定义 Invoke 行为；留空时返回 nil
	InvokeFunc func(ctx context.Context, req rpc.InvokeRequest) error
}

// 编译期验证接口实现
var _ rpc.RpcService = (*TestRpcService)(nil)

// NewTestRpcService 创建 TestRpcService 实例
func NewTestRpcService() *TestRpcService {
	return &TestRpcService{}
}

// Invoke 实现 RpcService.Invoke；BizHook 必填
func (m *TestRpcService) Invoke(ctx context.Context, req rpc.InvokeRequest) error {
	if req.BizHook == nil {
		return errors.New("rpctest: BizHook 不能为空")
	}
	if m.InvokeFunc != nil {
		return m.InvokeFunc(ctx, req)
	}
	return nil
}
