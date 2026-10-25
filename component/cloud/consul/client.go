// Package consul 封装 Consul 服务注册发现与负载均衡能力
//
// @author zwei
package consul

import (
	"net/http"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"

	"github.com/hashicorp/consul/api"
	"golang.org/x/sync/singleflight"
)

// Client 封装 Consul Agent API，提供服务注册与发现能力
type Client struct {
	api                *api.Client
	cacheTTL           time.Duration
	httpTimeout        time.Duration
	l1Cache            *serviceCache
	balancer           LoadBalancer
	discoveryURLScheme string // discoveryURLScheme 服务发现 URL 协议前缀，留空则不拼接

	sfGroup      singleflight.Group // sfGroup 合并同服务的并发发现查询
	negCache     *negativeCache     // negCache 缓存最近的发现失败，兼作后台再验证的探测限频
	staleIfError time.Duration      // staleIfError 是发现失败时陈旧列表的兜底窗口，0 表示关闭
}

// NewClient 根据配置创建 Consul 客户端；调用方应确保 cfg 已通过 Validate
func NewClient(cfg Config) (*Client, error) {
	httpTimeout := cfg.HTTPTimeoutOrDefault()
	apiClient, err := api.NewClient(&api.Config{
		Address: strings.TrimSpace(cfg.Addr),
		HttpClient: &http.Client{
			Timeout: httpTimeout,
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "创建 consul 客户端失败")
	}
	cacheTTL := cfg.DiscoveryCacheDuration()
	return &Client{
		api:                apiClient,
		cacheTTL:           cacheTTL,
		httpTimeout:        httpTimeout,
		l1Cache:            newServiceCache(cacheTTL),
		balancer:           NewRoundRobinBalancer(),
		discoveryURLScheme: cfg.DiscoveryURLSchemeClean(),
		negCache:           newNegativeCache(discoveryNegativeTTL),
		staleIfError:       cfg.DiscoveryStaleIfErrorOrDefault(),
	}, nil
}

// CacheTTL 返回当前服务发现缓存 TTL
func (c *Client) CacheTTL() time.Duration {
	return c.cacheTTL
}

// HTTPTimeout 返回访问 Consul Agent 的 HTTP 超时
func (c *Client) HTTPTimeout() time.Duration {
	return c.httpTimeout
}
