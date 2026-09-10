// Package wsbase 定义 Server/Client 构造配置
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
	// DefaultWriteMaxPayloadSize 单条逻辑消息默认可写上限（256KiB），映射 gws.WriteMaxPayloadSize
	DefaultWriteMaxPayloadSize = 256 * 1024

	// MaxWriteMaxPayloadSize 允许配置的单条可写上限（8MiB）
	MaxWriteMaxPayloadSize = 8 * 1024 * 1024

	// DefaultHeartbeatInterval 默认心跳 Ping 间隔
	DefaultHeartbeatInterval = 10 * time.Second

	// DefaultHeartbeatTimeout 默认读空闲超时（超时未获任意入站则断开）
	DefaultHeartbeatTimeout = 30 * time.Second

	// DefaultMaxConnLifetime 默认通道最大存活时间（从握手成功起算）
	DefaultMaxConnLifetime = 24 * time.Hour

	// DefaultMaxConnLifetimeJitter 默认存活时间波动半宽，用于打散同时到期
	DefaultMaxConnLifetimeJitter = 30 * time.Second
)

// Duration 返回 d 的指针，便于配置 HeartbeatInterval / HeartbeatTimeout /
// MaxConnLifetime / MaxConnLifetimeJitter
func Duration(d time.Duration) *time.Duration {
	return &d
}

// ServerConfig 独立端口 WebSocket 服务构造参数；由业务填充，本包不读配置文件
type ServerConfig struct {
	Name string // Name 是实例名，仅用于日志区分，可空
	Addr string // Addr 是监听地址，必填，如 ":7101" 或 "127.0.0.1:7101"

	CompressEnabled bool // CompressEnabled 是否启用 permessage-deflate
	ParallelEnabled bool // ParallelEnabled 是否并行处理消息
	CheckUTF8       bool // CheckUTF8 是否校验文本 UTF-8

	ReadMaxPayloadSize  int           // ReadMaxPayloadSize 单条逻辑消息可读最大载荷；0 用 gws 默认
	WriteMaxPayloadSize int           // WriteMaxPayloadSize 单条逻辑消息可写最大载荷；0 表示 DefaultWriteMaxPayloadSize
	HandshakeTimeout    time.Duration // HandshakeTimeout 握手超时；0 用 gws 默认

	// HeartbeatInterval 主动 Ping 间隔；nil 表示 DefaultHeartbeatInterval；显式 0 表示不发 Ping
	HeartbeatInterval *time.Duration
	// HeartbeatTimeout 读空闲超时；nil 表示 DefaultHeartbeatTimeout；显式 0 表示不超时踢连接
	// Timeout>0 时由本包接管该连接的 ReadDeadline；任意入站（消息/Ping/Pong）会刷新
	HeartbeatTimeout *time.Duration

	// MaxConnLifetime 单条通道最大存活时间，从握手成功起算，流量不续命。
	// nil 表示 DefaultMaxConnLifetime（24h）；显式 0 表示不限制
	MaxConnLifetime *time.Duration
	// MaxConnLifetimeJitter 存活时间波动半宽；实际寿命为基准加减该值的均匀随机。
	// nil 表示 DefaultMaxConnLifetimeJitter（30s）；显式 0 表示不波动。
	// 两者 resolve 后均 >0 时要求 Jitter < Lifetime
	MaxConnLifetimeJitter *time.Duration

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
	if err := validateWriteMaxPayloadSize(c.WriteMaxPayloadSize); err != nil {
		return err
	}
	if err := validateHeartbeat(c.HeartbeatInterval, c.HeartbeatTimeout); err != nil {
		return err
	}
	if err := validateMaxConnLifetime(c.MaxConnLifetime, c.MaxConnLifetimeJitter); err != nil {
		return err
	}
	return nil
}

// ResolvedWriteMaxPayloadSize 返回生效的单条可写上限
func (c ServerConfig) ResolvedWriteMaxPayloadSize() int {
	return resolveWriteMaxPayloadSize(c.WriteMaxPayloadSize)
}

// ResolvedHeartbeat 返回生效的心跳间隔与超时
func (c ServerConfig) ResolvedHeartbeat() (interval, timeout time.Duration) {
	return resolveHeartbeat(c.HeartbeatInterval, c.HeartbeatTimeout)
}

// ResolvedMaxConnLifetime 返回生效的通道最大存活时间与波动半宽
func (c ServerConfig) ResolvedMaxConnLifetime() (lifetime, jitter time.Duration) {
	return resolveMaxConnLifetime(c.MaxConnLifetime, c.MaxConnLifetimeJitter)
}

// ClientConfig 出站 WebSocket 客户端构造参数；由业务填充，本包不读配置文件
type ClientConfig struct {
	Name string // Name 是实例名，仅用于日志区分，可空

	CompressEnabled bool // CompressEnabled 是否启用 permessage-deflate
	ParallelEnabled bool // ParallelEnabled 是否并行处理消息
	CheckUTF8       bool // CheckUTF8 是否校验文本 UTF-8

	ReadMaxPayloadSize  int           // ReadMaxPayloadSize 单条逻辑消息可读最大载荷；0 用 gws 默认
	WriteMaxPayloadSize int           // WriteMaxPayloadSize 单条逻辑消息可写最大载荷；0 表示 DefaultWriteMaxPayloadSize
	HandshakeTimeout    time.Duration // HandshakeTimeout 握手超时；0 用 gws 默认

	// HeartbeatInterval 主动 Ping 间隔；nil 表示 DefaultHeartbeatInterval；显式 0 表示不发 Ping
	HeartbeatInterval *time.Duration
	// HeartbeatTimeout 读空闲超时；nil 表示 DefaultHeartbeatTimeout；显式 0 表示不超时踢连接
	HeartbeatTimeout *time.Duration

	RequestHeader http.Header // RequestHeader 握手额外请求头（如鉴权）
	TLSConfig     *tls.Config // TLSConfig wss 时可选 TLS 配置

	// NewDialer 自定义拨号（代理等）；nil 使用默认
	NewDialer func() (gws.Dialer, error)

	// CustomizeGWS 在填充完常用字段后回调，用于未封装的 gws.ClientOption
	CustomizeGWS func(opt *gws.ClientOption)
}

// Validate 校验 ClientConfig
func (c ClientConfig) Validate() error {
	if err := validateWriteMaxPayloadSize(c.WriteMaxPayloadSize); err != nil {
		return err
	}
	if err := validateHeartbeat(c.HeartbeatInterval, c.HeartbeatTimeout); err != nil {
		return err
	}
	return nil
}

// ResolvedWriteMaxPayloadSize 返回生效的单条可写上限
func (c ClientConfig) ResolvedWriteMaxPayloadSize() int {
	return resolveWriteMaxPayloadSize(c.WriteMaxPayloadSize)
}

// ResolvedHeartbeat 返回生效的心跳间隔与超时
func (c ClientConfig) ResolvedHeartbeat() (interval, timeout time.Duration) {
	return resolveHeartbeat(c.HeartbeatInterval, c.HeartbeatTimeout)
}

// validateWriteMaxPayloadSize 校验单条可写上限；0 表示使用默认值故合法
func validateWriteMaxPayloadSize(n int) error {
	if n < 0 {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "WriteMaxPayloadSize 不能为负数")
	}
	if n > MaxWriteMaxPayloadSize {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "WriteMaxPayloadSize 超过 MaxWriteMaxPayloadSize")
	}
	return nil
}

// resolveWriteMaxPayloadSize 将 0/未设置解析为 DefaultWriteMaxPayloadSize
func resolveWriteMaxPayloadSize(n int) int {
	if n <= 0 {
		return DefaultWriteMaxPayloadSize
	}
	return n
}

// resolveHeartbeat 解析心跳：nil→默认；显式 0 保留为关闭对应能力
func resolveHeartbeat(interval, timeout *time.Duration) (time.Duration, time.Duration) {
	var i, t time.Duration
	if interval == nil {
		i = DefaultHeartbeatInterval
	} else {
		i = *interval
	}
	if timeout == nil {
		t = DefaultHeartbeatTimeout
	} else {
		t = *timeout
	}
	return i, t
}

// validateHeartbeat 校验心跳配置；两者均 >0 时要求 Timeout > Interval
func validateHeartbeat(interval, timeout *time.Duration) error {
	if interval != nil && *interval < 0 {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "HeartbeatInterval 不能为负数")
	}
	if timeout != nil && *timeout < 0 {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "HeartbeatTimeout 不能为负数")
	}
	i, t := resolveHeartbeat(interval, timeout)
	if i > 0 && t > 0 && t <= i {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "HeartbeatTimeout 必须大于 HeartbeatInterval")
	}
	return nil
}

// resolveMaxConnLifetime 解析通道寿命：nil→默认；显式 0 保留为关闭对应能力
func resolveMaxConnLifetime(lifetime, jitter *time.Duration) (time.Duration, time.Duration) {
	var life, jit time.Duration
	if lifetime == nil {
		life = DefaultMaxConnLifetime
	} else {
		life = *lifetime
	}
	if jitter == nil {
		jit = DefaultMaxConnLifetimeJitter
	} else {
		jit = *jitter
	}
	return life, jit
}

// validateMaxConnLifetime 校验通道寿命；两者均 >0 时要求 Jitter < Lifetime
func validateMaxConnLifetime(lifetime, jitter *time.Duration) error {
	if lifetime != nil && *lifetime < 0 {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "MaxConnLifetime 不能为负数")
	}
	if jitter != nil && *jitter < 0 {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "MaxConnLifetimeJitter 不能为负数")
	}
	life, jit := resolveMaxConnLifetime(lifetime, jitter)
	if life > 0 && jit > 0 && jit >= life {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "MaxConnLifetimeJitter 必须小于 MaxConnLifetime")
	}
	return nil
}

// toGWSOption 映射为 gws.ServerOption
func (c ServerConfig) toGWSOption() *gws.ServerOption {
	opt := &gws.ServerOption{
		PermessageDeflate:   gws.PermessageDeflate{Enabled: c.CompressEnabled},
		ParallelEnabled:     c.ParallelEnabled,
		CheckUtf8Enabled:    c.CheckUTF8,
		ReadMaxPayloadSize:  c.ReadMaxPayloadSize,
		WriteMaxPayloadSize: c.ResolvedWriteMaxPayloadSize(),
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
		WriteMaxPayloadSize: c.ResolvedWriteMaxPayloadSize(),
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
