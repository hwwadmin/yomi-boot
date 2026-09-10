// Package wsbase 实现出站 WebSocket Client
//
// @author zwei
package wsbase

import (
	"context"
	"net/http"

	"gitee.com/hq_project/yomi-boot/errors"

	"github.com/lxzan/gws"
)

// Client 是出站 WebSocket 客户端工厂；可 New 多个，Dial 得到 Conn 后需 go Serve()
type Client struct {
	name      string
	chunkSize int
	cfg       ClientConfig
	adapter   *eventAdapter
}

// NewClient 根据配置创建 Client
func NewClient(h Handler, cfg ClientConfig) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Client{
		name:      cfg.Name,
		chunkSize: cfg.ResolvedChunkSize(),
		cfg:       cfg,
		adapter:   newEventAdapter(h, cfg.ResolvedChunkSize()),
	}, nil
}

// Name 返回实例名
func (c *Client) Name() string {
	if c == nil {
		return ""
	}
	return c.name
}

// Dial 拨号建立连接；成功后业务须 go conn.Serve() 启动读循环
func (c *Client) Dial(ctx context.Context, url string) (Conn, *http.Response, error) {
	if c == nil {
		return nil, nil, errors.Sentinel(ErrInvalidArgument)
	}
	if url == "" {
		return nil, nil, errors.Wrap(errors.Sentinel(ErrInvalidArgument), "Dial url 不能为空")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	opt := c.cfg.toGWSOption(url)
	raw, resp, err := gws.NewClient(c.adapter, opt)
	if err != nil {
		return nil, resp, errors.Wrap(err, "ws Dial 失败")
	}
	conn := connFromGWS(raw, c.chunkSize)
	return conn, resp, nil
}
