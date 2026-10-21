package rpctest

import (
	"context"
	"testing"

	"gitee.com/hq_project/yomi-boot/cloud/rpc"
)

// TestTestRpcService_Invoke 验证测试桩透传参数与零值可用
func TestTestRpcService_Invoke(t *testing.T) {
	var gotEndpoint string
	s := &TestRpcService{InvokeFunc: func(_ context.Context, endpoint string, _ any, _ any, _ rpc.BizHookFunc) error {
		gotEndpoint = endpoint
		return nil
	}}
	if err := s.Invoke(context.Background(), "/x", nil, nil, rpc.NoopBizHook); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if gotEndpoint != "/x" {
		t.Fatalf("endpoint: got %q want /x", gotEndpoint)
	}

	if err := (&TestRpcService{}).Invoke(context.Background(), "/x", nil, nil, rpc.NoopBizHook); err != nil {
		t.Fatalf("零值 Invoke 应返回 nil，实际 %v", err)
	}
	if err := (&TestRpcService{}).Invoke(context.Background(), "/x", nil, nil, nil); err == nil {
		t.Fatal("nil bizHook 应返回错误")
	}
}
