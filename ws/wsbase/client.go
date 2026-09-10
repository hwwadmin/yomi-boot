// Package wsbase 实现出站 WebSocket Client
//
// @author zwei
package wsbase

import (
	"context"
	"net/http"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"

	"github.com/lxzan/gws"
)

// Client 是出站 WebSocket 客户端工厂；可 New 多个，Dial 得到 Conn 后需 go Serve()
type Client struct {
	name    string
	cfg     ClientConfig
	adapter *eventAdapter
}

// dialResult 是异步拨号结果
type dialResult struct {
	conn Conn
	resp *http.Response
	err  error
}

// NewClient 根据配置创建 Client
func NewClient(h Handler, cfg ClientConfig) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	interval, timeout := cfg.ResolvedHeartbeat()
	return &Client{
		name:    cfg.Name,
		cfg:     cfg,
		adapter: newEventAdapter(h, heartbeatFromResolved(interval, timeout)),
	}, nil
}

// Name 返回实例名
func (c *Client) Name() string {
	if c == nil {
		return ""
	}
	return c.name
}

// Dial 拨号建立连接；成功后业务须 go conn.Serve() 启动读循环。
// ctx 取消或超时时尽快返回；若拨号晚于取消完成，会关闭已建立连接以免泄漏。
func (c *Client) Dial(ctx context.Context, url string) (Conn, *http.Response, error) {
	if c == nil {
		return nil, nil, errors.Sentinel(ErrInvalidArgument)
	}
	if url == "" {
		return nil, nil, errors.Wrap(errors.Sentinel(ErrInvalidArgument), "Dial url 不能为空")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	handshakeTimeout := effectiveHandshakeTimeout(ctx, c.cfg.ResolvedHandshakeTimeout())
	opt := c.cfg.toGWSOption(url, handshakeTimeout)

	ch := make(chan dialResult, 1)
	go func() {
		raw, resp, err := gws.NewClient(c.adapter, opt)
		if err != nil {
			ch <- dialResult{resp: resp, err: errors.Wrap(err, "ws Dial 失败")}
			return
		}
		ch <- dialResult{conn: connFromGWS(raw), resp: resp}
	}()

	select {
	case <-ctx.Done():
		go discardDialResult(ch)
		return nil, nil, ctx.Err()
	case r := <-ch:
		if err := ctx.Err(); err != nil {
			if r.conn != nil {
				_ = r.conn.Close(CloseGoingAway, "context canceled")
			}
			return nil, r.resp, err
		}
		return r.conn, r.resp, r.err
	}
}

// effectiveHandshakeTimeout 取配置握手超时与 ctx 剩余截止时间的较早者
func effectiveHandshakeTimeout(ctx context.Context, base time.Duration) time.Duration {
	if base <= 0 {
		base = DefaultHandshakeTimeout
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return base
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return time.Nanosecond
	}
	if remaining < base {
		return remaining
	}
	return base
}

// discardDialResult 在 ctx 已取消后回收晚到的拨号结果，避免连接泄漏
func discardDialResult(ch <-chan dialResult) {
	r := <-ch
	if r.conn != nil {
		_ = r.conn.Close(CloseGoingAway, "context canceled")
	}
}
