// @author zwei

package consul

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// ConsulRPCResolver 将 Consul 服务发现适配为 RPC 端点解析器
type ConsulRPCResolver struct {
	client *Client
}

// NewConsulRPCResolver 创建 Consul RPC 端点解析适配器
func NewConsulRPCResolver(client *Client) *ConsulRPCResolver {
	return &ConsulRPCResolver{client: client}
}

// ResolveHTTPBase 解析服务实例并拼接路径
func (r *ConsulRPCResolver) ResolveHTTPBase(ctx context.Context, serviceName, pathSuffix string) (string, error) {
	if r == nil || r.client == nil {
		return "", errors.New("ConsulRPCResolver 未初始化")
	}
	return r.client.ResolveHTTPBase(ctx, serviceName, pathSuffix)
}
