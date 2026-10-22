// Package dtm 提供 DTM Saga 分布式事务引擎集成能力
//
// @author zwei
package dtm

import (
	"net/url"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/component/cloud/consul"
	"gitee.com/hq_project/yomi-boot/core/errors"
)

// DefaultTimeout 是 DTM 控制面请求（newGid/submit/abort/query）的默认超时
// 该值同时作为 DTM 服务端回调各分支的请求超时
const DefaultTimeout = 3 * time.Second

// ErrNotConfigured 表示 DTM 服务端未配置
var ErrNotConfigured = errors.NewNoStack("dtm server 未配置")

// ErrGenGidFailed 表示向 DTM 请求全局事务 ID 失败
var ErrGenGidFailed = errors.NewNoStack("dtm 生成全局事务 ID 失败")

// Client 是进程级 DTM 客户端，持有服务地址、服务发现客户端与请求超时
// server 为空表示未启用；此时 NewSaga/AddSagaStep 返回 ErrNotConfigured
type Client struct {
	consul  *consul.Client
	server  string
	timeout time.Duration
}

// Option 是 Client 的可选配置
type Option func(*Client)

// WithTimeout 设置控制面请求超时，timeout <= 0 时使用 DefaultTimeout
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		c.timeout = timeout
	}
}

// NewClient 创建 DTM 客户端，应在启动装配阶段调用一次
// consulClient 仅在 AddSagaStep 解析服务地址时使用，可为 nil；server 为空表示未启用
// server 非空时必须带 http/https 协议与 host，否则 panic
func NewClient(consulClient *consul.Client, server string, opts ...Option) *Client {
	server = strings.TrimRight(strings.TrimSpace(server), "/")
	if server != "" {
		if err := validateHTTPURL("server", server); err != nil {
			panic(err)
		}
	}
	c := &Client{consul: consulClient, server: server, timeout: DefaultTimeout}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	if c.timeout <= 0 {
		c.timeout = DefaultTimeout
	}
	return c
}

// Enabled 返回是否配置了 DTM 服务端地址
func (c *Client) Enabled() bool {
	return c != nil && c.server != ""
}

// effectiveTimeout 返回实际生效的控制面超时，零值客户端也退回默认值
func (c *Client) effectiveTimeout() time.Duration {
	if c == nil || c.timeout <= 0 {
		return DefaultTimeout
	}
	return c.timeout
}

// validateHTTPURL 校验 DTM 服务地址或分支地址是带 http/https 与 host 的绝对地址
func validateHTTPURL(kind, raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return errors.Wrap(err, "dtm: %s 地址不合法", kind)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.Newf("dtm: %s 地址不合法: %s", kind, raw)
	}
	return nil
}
