package rpctest

import (
	"context"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/cloud/rpc"
)

// TestTestRpcService_Invoke 验证测试桩透传参数与零值可用
func TestTestRpcService_Invoke(t *testing.T) {
	var gotEndpoint string
	s := &TestRpcService{InvokeFunc: func(_ context.Context, req rpc.InvokeRequest) error {
		gotEndpoint = req.Endpoint
		return nil
	}}
	if err := s.Invoke(context.Background(), rpc.InvokeRequest{Endpoint: "/x", BizHook: rpc.NoopBizHook}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if gotEndpoint != "/x" {
		t.Fatalf("endpoint: got %q want /x", gotEndpoint)
	}

	if err := (&TestRpcService{}).Invoke(context.Background(), rpc.InvokeRequest{Endpoint: "/x", BizHook: rpc.NoopBizHook}); err != nil {
		t.Fatalf("零值 Invoke 应返回 nil，实际 %v", err)
	}
	if err := (&TestRpcService{}).Invoke(context.Background(), rpc.InvokeRequest{Endpoint: "/x"}); err == nil {
		t.Fatal("nil BizHook 应返回错误")
	}
}
