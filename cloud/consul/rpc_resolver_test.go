package consul

import (
	"context"
	"testing"

	"gitee.com/hq_project/yomi-boot/cloud/rpc"
)

// RPCResolver 必须满足 rpc.EndpointResolver
var _ rpc.EndpointResolver = (*RPCResolver)(nil)

// TestRPCResolverNilClient 验证适配器对 nil 客户端的防御
func TestRPCResolverNilClient(t *testing.T) {
	if _, err := NewRPCResolver(nil).ResolveHTTPBase(context.Background(), "svc", "/x"); err == nil {
		t.Fatal("nil client 应返回错误")
	}
}
