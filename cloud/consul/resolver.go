// Package consul 提供服务发现与 HTTP 地址解析：一级进程缓存 + 二级 Agent 查询
package consul

import (
	"context"
	"fmt"
	"strings"

	"gitee.com/hq_project/yomi-boot/core/errors"

	"github.com/hashicorp/consul/api"
)

// ResolveHTTPBase 发现健康的服务实例，返回带路径后缀的基 URL。
// 一级缓存：进程内实例列表；二级缓存：Consul Agent（UseCache + max-age）。
func (c *Client) ResolveHTTPBase(ctx context.Context, serviceName, pathSuffix string) (string, error) {
	// 校验上下文是否已取消
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// 校验服务名非空
	name := strings.TrimSpace(serviceName)
	if name == "" {
		return "", errors.New("服务名不能为空")
	}

	// 获取健康实例列表（优先一级缓存）
	instances, err := c.instances(ctx, name)
	if err != nil {
		return "", err
	}
	// 通过负载均衡器选择一个实例
	picked, err := c.balancer.Pick(name, instances)
	if err != nil {
		return "", err
	}
	// 格式化为服务基 URL
	return c.formatHTTPBase(picked, pathSuffix), nil
}

// instances 返回指定服务的健康实例列表（优先使用一级缓存）
func (c *Client) instances(ctx context.Context, serviceName string) ([]ServiceInstance, error) {
	if instances, ok := c.l1Cache.get(serviceName); ok {
		return instances, nil
	}
	instances, err := c.fetchHealthyInstances(ctx, serviceName)
	if err != nil {
		return nil, err
	}
	c.l1Cache.set(serviceName, instances)
	return instances, nil
}

// fetchHealthyInstances 从 Consul Agent 查询健康的服务实例
func (c *Client) fetchHealthyInstances(ctx context.Context, serviceName string) ([]ServiceInstance, error) {
	// 查询 Consul Agent 健康服务（启用本地 Agent 缓存）
	entries, _, err := c.api.Health().Service(serviceName, "", true, (&api.QueryOptions{
		UseCache: true,
		MaxAge:   c.cacheTTL,
	}).WithContext(ctx))
	if err != nil {
		return nil, errors.Wrap(err, "consul 发现 %s 失败", serviceName)
	}
	if len(entries) == 0 {
		return nil, errors.Newf("consul 未发现健康实例: %s", serviceName)
	}
	// 遍历结果提取有效地址与端口
	instances := make([]ServiceInstance, 0, len(entries))
	for _, entry := range entries {
		// 优先使用 Service.Address，为空时回退到 Node.Address
		addr := strings.TrimSpace(entry.Service.Address)
		if addr == "" {
			addr = strings.TrimSpace(entry.Node.Address)
		}
		if addr == "" {
			continue
		}
		port := entry.Service.Port
		if port <= 0 {
			continue
		}
		instances = append(instances, ServiceInstance{Address: addr, Port: port})
	}
	if len(instances) == 0 {
		return nil, errors.Newf("consul 实例地址无效: %s", serviceName)
	}
	return instances, nil
}

// formatHTTPBase 将服务实例的地址和端口拼接为基 URL；配置了 discovery_url_scheme 时追加协议前缀
func (c *Client) formatHTTPBase(instance ServiceInstance, pathSuffix string) string {
	suffix := strings.TrimRight(pathSuffix, "/")
	if suffix != "" && !strings.HasPrefix(suffix, "/") {
		suffix = "/" + suffix
	}
	base := fmt.Sprintf("%s:%d%s", instance.Address, instance.Port, suffix)
	if c.discoveryURLScheme == "" {
		return base
	}
	return c.discoveryURLScheme + base
}
