package consul

import (
	"context"
	"testing"

	"gitee.com/hq_project/yomi-boot/cloud/rpc"
)

// 编译期验证 RPC 端点解析接口实现
var _ rpc.EndpointResolver = (*ConsulRPCResolver)(nil)

// TestConsulRPCResolverNilClient 验证适配器对 nil 客户端的防御
func TestConsulRPCResolverNilClient(t *testing.T) {
	if _, err := NewConsulRPCResolver(nil).ResolveHTTPBase(context.Background(), "svc", "/x"); err == nil {
		t.Fatal("nil client 应返回错误")
	}
}
