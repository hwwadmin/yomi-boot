// Package consul 提供服务发现与 HTTP 地址解析：一级进程缓存 + 二级 Agent 查询
package consul

import (
	"context"
	"fmt"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/logger"

	"github.com/hashicorp/consul/api"
)

// errNoHealthyInstances 表示 Agent 查询成功但服务没有可用健康实例；
// 属确定性结果，不做 stale 兜底，避免把流量路由到已下线实例
var errNoHealthyInstances = errors.NewNoStack("consul: 无健康实例")

// ResolveHTTPBase 发现健康的服务实例，返回带路径后缀的基 URL。
// 一级缓存：进程内实例列表；二级缓存：Consul Agent（UseCache + max-age）。
// Agent 故障时在 stale-if-error 窗口内返回陈旧列表并转后台再验证。
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

// instances 返回指定服务的健康实例列表。
// 优先一级缓存；过期条目在 stale-if-error 窗口内可直接兜底（转后台再验证刷新）；
// 无陈旧可用时经 singleflight 合并并发查询，失败结果短暂负缓存，避免断连期间
// 每个请求都阻塞在 Agent 查询上
func (c *Client) instances(ctx context.Context, serviceName string) ([]ServiceInstance, error) {
	if instances, ok := c.l1Cache.get(serviceName); ok {
		return instances, nil
	}
	// Consul 故障期间兜底返回陈旧列表，调用方不阻塞；由后台再验证探测恢复
	if instances, ok := c.l1Cache.getStale(serviceName, c.staleIfError); ok {
		c.revalidateAsync(ctx, serviceName)
		return instances, nil
	}
	// 负缓存窗口内直接返回最近一次失败，不再探测
	if err, ok := c.negCache.get(serviceName); ok {
		return nil, err
	}
	v, err, _ := c.sfGroup.Do(serviceName, func() (any, error) {
		return c.fetchAndCache(ctx, serviceName)
	})
	if err != nil {
		c.negCache.set(serviceName, err)
		return nil, err
	}
	instances, _ := v.([]ServiceInstance)
	return instances, nil
}

// fetchAndCache 查询 Agent 并写入一级缓存；先查缓存去重，与后台再验证共用 singleflight 自然合并
func (c *Client) fetchAndCache(ctx context.Context, serviceName string) (any, error) {
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

// revalidateAsync 后台重新解析以刷新陈旧列表；经 negativeCache 限频，
// 故障期间探测频率不超过每窗口一次。成功静默刷新缓存，失败记录负缓存；
// 确认无健康实例时清除陈旧条目，让后续请求快速失败而不是路由到已下线实例
func (c *Client) revalidateAsync(ctx context.Context, serviceName string) {
	if _, ok := c.negCache.get(serviceName); ok {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.ErrorWithoutCtx("consul 发现再验证 goroutine panic",
					"service", serviceName, "panic", r)
			}
		}()
		// 探测不绑定调用方 ctx：调用方返回后仍要完成刷新
		probeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.httpTimeout)
		defer cancel()
		_, err, _ := c.sfGroup.Do(serviceName, func() (any, error) {
			return c.fetchAndCache(probeCtx, serviceName)
		})
		if err == nil {
			logger.Info(ctx, "consul 发现再验证成功，实例列表已刷新", "service", serviceName)
			return
		}
		c.negCache.set(serviceName, err)
		if errors.Is(err, errNoHealthyInstances) {
			c.l1Cache.delete(serviceName)
			logger.Warn(ctx, "consul 发现确认无健康实例，陈旧列表已失效", "service", serviceName)
			return
		}
		logger.Warn(ctx, "consul 发现再验证失败，继续使用陈旧实例列表",
			"service", serviceName, "error", err)
	}()
}

// fetchHealthyInstances 从 Consul Agent 查询健康的服务实例
func (c *Client) fetchHealthyInstances(ctx context.Context, serviceName string) ([]ServiceInstance, error) {
	// 查询 Consul Agent 健康服务（启用本地 Agent 缓存；servers 失联时允许 Agent 回陈旧数据）
	entries, _, err := c.api.Health().Service(serviceName, "", true, (&api.QueryOptions{
		UseCache:     true,
		MaxAge:       c.cacheTTL,
		StaleIfError: c.staleIfError,
	}).WithContext(ctx))
	if err != nil {
		return nil, errors.Wrap(err, "consul 发现 %s 失败", serviceName)
	}
	if len(entries) == 0 {
		return nil, errors.Wrap(errNoHealthyInstances, "consul 未发现健康实例: %s", serviceName)
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
		return nil, errors.Wrap(errNoHealthyInstances, "consul 实例地址无效: %s", serviceName)
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
