// Package wsbase 定义 Server/Client 构造配置与分片默认值
//
// @author zwei
package wsbase

import (
	"crypto/tls"
	"net/http"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"

	"github.com/lxzan/gws"
)

const (
	// DefaultChunkSize 大负载 WriteReader 默认分片大小（256KiB）
	DefaultChunkSize = 256 * 1024

	// MaxChunkSize 允许配置的分片上限（8MiB），防止误配成近乎整文件一片
	MaxChunkSize = 8 * 1024 * 1024
)

// ServerConfig 独立端口 WebSocket 服务构造参数；由业务填充，本包不读配置文件
type ServerConfig struct {
	Name string // Name 是实例名，仅用于日志区分，可空
	Addr string // Addr 是监听地址，必填，如 ":7101" 或 "127.0.0.1:7101"

	ChunkSize int // ChunkSize 是本实例 WriteReader 默认分片字节数；0 表示 DefaultChunkSize

	CompressEnabled bool // CompressEnabled 是否启用 permessage-deflate
	ParallelEnabled bool // ParallelEnabled 是否并行处理消息
	CheckUTF8       bool // CheckUTF8 是否校验文本 UTF-8

	ReadMaxPayloadSize  int           // ReadMaxPayloadSize 单条可读最大载荷；0 用 gws 默认
	WriteMaxPayloadSize int           // WriteMaxPayloadSize 单条可写最大载荷；0 用 gws 默认
	HandshakeTimeout    time.Duration // HandshakeTimeout 握手超时；0 用 gws 默认

	TLSConfig *tls.Config                                 // TLSConfig 供 StartTLS 合并（证书仍由 StartTLS 参数加载）
	Authorize func(r *http.Request, session Session) bool // Authorize 握手鉴权；nil 表示放行

	// CustomizeGWS 在填充完常用字段后回调，用于未封装的 gws.ServerOption
	CustomizeGWS func(opt *gws.ServerOption)
}

// Validate 校验 ServerConfig
func (c ServerConfig) Validate() error {
	if c.Addr == "" {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "ServerConfig.Addr 不能为空")
	}
	if err := validateChunkSize(c.ChunkSize); err != nil {
		return err
	}
	return nil
}

// ResolvedChunkSize 返回生效的默认分片大小
func (c ServerConfig) ResolvedChunkSize() int {
	return resolveChunkSize(c.ChunkSize)
}

// ClientConfig 出站 WebSocket 客户端构造参数；由业务填充，本包不读配置文件
type ClientConfig struct {
	Name string // Name 是实例名，仅用于日志区分，可空

	ChunkSize int // ChunkSize 是本实例 WriteReader 默认分片字节数；0 表示 DefaultChunkSize

	CompressEnabled bool // CompressEnabled 是否启用 permessage-deflate
	ParallelEnabled bool // ParallelEnabled 是否并行处理消息
	CheckUTF8       bool // CheckUTF8 是否校验文本 UTF-8

	ReadMaxPayloadSize  int           // ReadMaxPayloadSize 单条可读最大载荷；0 用 gws 默认
	WriteMaxPayloadSize int           // WriteMaxPayloadSize 单条可写最大载荷；0 用 gws 默认
	HandshakeTimeout    time.Duration // HandshakeTimeout 握手超时；0 用 gws 默认

	RequestHeader http.Header // RequestHeader 握手额外请求头（如鉴权）
	TLSConfig     *tls.Config // TLSConfig wss 时可选 TLS 配置

	// NewDialer 自定义拨号（代理等）；nil 使用默认
	NewDialer func() (gws.Dialer, error)

	// CustomizeGWS 在填充完常用字段后回调，用于未封装的 gws.ClientOption
	CustomizeGWS func(opt *gws.ClientOption)
}

// Validate 校验 ClientConfig
func (c ClientConfig) Validate() error {
	if err := validateChunkSize(c.ChunkSize); err != nil {
		return err
	}
	return nil
}

// ResolvedChunkSize 返回生效的默认分片大小
func (c ClientConfig) ResolvedChunkSize() int {
	return resolveChunkSize(c.ChunkSize)
}

// validateChunkSize 校验分片大小；0 表示使用默认值故合法
func validateChunkSize(n int) error {
	if n < 0 {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "ChunkSize 不能为负数")
	}
	if n > MaxChunkSize {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "ChunkSize 超过 MaxChunkSize")
	}
	return nil
}

// resolveChunkSize 将 0/未设置解析为 DefaultChunkSize
func resolveChunkSize(n int) int {
	if n <= 0 {
		return DefaultChunkSize
	}
	return n
}

// toGWSOption 映射为 gws.ServerOption
func (c ServerConfig) toGWSOption() *gws.ServerOption {
	opt := &gws.ServerOption{
		PermessageDeflate:   gws.PermessageDeflate{Enabled: c.CompressEnabled},
		ParallelEnabled:     c.ParallelEnabled,
		CheckUtf8Enabled:    c.CheckUTF8,
		ReadMaxPayloadSize:  c.ReadMaxPayloadSize,
		WriteMaxPayloadSize: c.WriteMaxPayloadSize,
		HandshakeTimeout:    c.HandshakeTimeout,
		TlsConfig:           c.TLSConfig,
		Recovery:            gws.Recovery,
	}
	if c.Authorize != nil {
		opt.Authorize = func(r *http.Request, ss gws.SessionStorage) bool {
			return c.Authorize(r, sessionFromGWS(ss))
		}
	}
	if c.CustomizeGWS != nil {
		c.CustomizeGWS(opt)
	}
	return opt
}

// toGWSOption 映射为 gws.ClientOption
func (c ClientConfig) toGWSOption(addr string) *gws.ClientOption {
	opt := &gws.ClientOption{
		Addr:                addr,
		PermessageDeflate:   gws.PermessageDeflate{Enabled: c.CompressEnabled},
		ParallelEnabled:     c.ParallelEnabled,
		CheckUtf8Enabled:    c.CheckUTF8,
		ReadMaxPayloadSize:  c.ReadMaxPayloadSize,
		WriteMaxPayloadSize: c.WriteMaxPayloadSize,
		HandshakeTimeout:    c.HandshakeTimeout,
		RequestHeader:       c.RequestHeader,
		TlsConfig:           c.TLSConfig,
		NewDialer:           c.NewDialer,
		Recovery:            gws.Recovery,
	}
	if c.CustomizeGWS != nil {
		c.CustomizeGWS(opt)
	}
	return opt
}
