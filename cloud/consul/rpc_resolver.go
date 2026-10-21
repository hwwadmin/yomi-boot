// @author zwei

package consul

import (
	"context"

	"gitee.com/hq_project/yomi-boot/boot/errors"
)

// RPCResolver 将 Consul 服务发现适配为 RPC 端点解析器
type RPCResolver struct {
	client *Client
}

// NewRPCResolver 创建 RPC 端点解析适配器
func NewRPCResolver(client *Client) *RPCResolver {
	return &RPCResolver{client: client}
}

// ResolveHTTPBase 解析服务实例并拼接路径
func (r *RPCResolver) ResolveHTTPBase(ctx context.Context, serviceName, pathSuffix string) (string, error) {
	if r == nil || r.client == nil {
		return "", errors.New("consul RPCResolver 未初始化")
	}
	return r.client.ResolveHTTPBase(ctx, serviceName, pathSuffix)
}
