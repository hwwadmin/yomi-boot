// Package ws 定义 Server/Client 构造配置
//
// @author zwei
package ws

import (
	"crypto/tls"
	"net/http"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"
)

const (
	// DefaultWriteMaxPayloadSize 单条逻辑消息默认可写上限（256KiB）
	DefaultWriteMaxPayloadSize = 256 * 1024

	// DefaultReadMaxPayloadSize 单条逻辑消息默认可读上限（256KiB）
	DefaultReadMaxPayloadSize = DefaultWriteMaxPayloadSize

	// MaxWriteMaxPayloadSize 允许配置的单条可写/可读上限（8MiB）
	MaxWriteMaxPayloadSize = 8 * 1024 * 1024

	// DefaultServerHeartbeatInterval 服务端默认心跳 Ping 间隔（0 = 不主动发 Ping）
	DefaultServerHeartbeatInterval = time.Duration(0)

	// DefaultClientHeartbeatInterval 客户端默认心跳 Ping 间隔
	DefaultClientHeartbeatInterval = 10 * time.Second

	// DefaultHeartbeatInterval 是 DefaultClientHeartbeatInterval 的别名
	DefaultHeartbeatInterval = DefaultClientHeartbeatInterval

	// DefaultHeartbeatTimeout 默认读空闲超时；Server/Client 的 Timeout 为 nil 时均使用该值
	DefaultHeartbeatTimeout = 30 * time.Second

	// DefaultMaxConnLifetime 默认通道最大存活时间（从握手成功起算）
	DefaultMaxConnLifetime = 24 * time.Hour

	// DefaultMaxConnLifetimeJitter 默认存活时间波动半宽，用于打散同时到期
	DefaultMaxConnLifetimeJitter = 30 * time.Second

	// DefaultHandshakeTimeout 默认握手超时（读入站请求头 + 升级写出）
	DefaultHandshakeTimeout = 5 * time.Second
)

// Duration 返回 d 的指针，便于配置 HeartbeatInterval / HeartbeatTimeout /
// MaxConnLifetime / MaxConnLifetimeJitter
func Duration(d time.Duration) *time.Duration {
	return &d
}

// ServerConfig 独立端口 WebSocket 服务构造参数；由业务填充，本包不读配置文件。
//
// Handler / Processor 接线规则：
//   - Handler == nil：使用 DefaultServerHandler，其 OnPayload 消费 Processor
//   - Handler != nil：使用自定义 Handler；Config.Processor 不自动接入
//     （自定义 Handler 若仍需 Processor，可嵌入 DefaultServerHandler 自行持有）
type ServerConfig struct {
	Name string // Name 是实例名，仅用于日志区分，可空
	Addr string // Addr 是监听地址，必填，如 ":7101" 或 "127.0.0.1:7101"

	CompressEnabled bool // CompressEnabled 是否启用 permessage-deflate

	ReadMaxPayloadSize  int           // ReadMaxPayloadSize 单条逻辑消息可读最大载荷；0 表示 DefaultReadMaxPayloadSize
	WriteMaxPayloadSize int           // WriteMaxPayloadSize 单条逻辑消息可写最大载荷；0 表示 DefaultWriteMaxPayloadSize
	HandshakeTimeout    time.Duration // HandshakeTimeout 握手超时；0 表示 DefaultHandshakeTimeout

	// HeartbeatInterval 主动 Ping 间隔；nil 表示 DefaultServerHeartbeatInterval；显式 0 表示不发 Ping
	HeartbeatInterval *time.Duration
	// HeartbeatTimeout 读空闲超时；nil 表示 DefaultHeartbeatTimeout；显式 0 表示不超时踢连接
	HeartbeatTimeout *time.Duration

	// MaxConnLifetime 单条通道最大存活时间，从握手成功起算，流量不续命。
	// nil 表示 DefaultMaxConnLifetime（24h）；显式 0 表示不限制
	MaxConnLifetime *time.Duration
	// MaxConnLifetimeJitter 存活时间波动半宽；实际寿命为基准加减该值的均匀随机。
	// nil 表示 DefaultMaxConnLifetimeJitter（30s）；显式 0 表示不波动。
	// 两者 resolve 后均 >0 时要求 Jitter < Lifetime
	MaxConnLifetimeJitter *time.Duration

	// MaxConnections 同时在线连接上限；0 表示不限制；超过时握手成功后立即以 CloseTryAgainLater 断开
	MaxConnections int

	// OriginPatterns 允许的 Origin host 模式（path.Match）；空则不做 Origin 校验
	OriginPatterns []string

	TLSConfig *tls.Config                // TLSConfig 供 StartTLS 合并（证书仍由 StartTLS 参数加载）
	Authorize func(r *http.Request) bool // Authorize 握手鉴权；nil 表示放行

	Handler   ServerHandler          // Handler 可选；nil 则用默认实现
	Processor ServerMessageProcessor // Processor 可选；仅 Handler 为 nil 时由默认实现消费
}

// Validate 校验 ServerConfig
func (c ServerConfig) Validate() error {
	if c.Addr == "" {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "ServerConfig.Addr 不能为空")
	}
	if err := validatePayloadSize(c.WriteMaxPayloadSize, "WriteMaxPayloadSize"); err != nil {
		return err
	}
	if err := validatePayloadSize(c.ReadMaxPayloadSize, "ReadMaxPayloadSize"); err != nil {
		return err
	}
	if err := validateHeartbeat(c.HeartbeatInterval, c.HeartbeatTimeout, resolveServerHeartbeat); err != nil {
		return err
	}
	if err := validateMaxConnLifetime(c.MaxConnLifetime, c.MaxConnLifetimeJitter); err != nil {
		return err
	}
	if c.MaxConnections < 0 {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "MaxConnections 不能为负数")
	}
	return nil
}

// ResolvedWriteMaxPayloadSize 返回生效的单条可写上限
func (c ServerConfig) ResolvedWriteMaxPayloadSize() int {
	return resolveWriteMaxPayloadSize(c.WriteMaxPayloadSize)
}

// ResolvedReadMaxPayloadSize 返回生效的单条可读上限
func (c ServerConfig) ResolvedReadMaxPayloadSize() int {
	return resolveReadMaxPayloadSize(c.ReadMaxPayloadSize)
}

// ResolvedHandshakeTimeout 返回生效的握手超时
func (c ServerConfig) ResolvedHandshakeTimeout() time.Duration {
	return resolveHandshakeTimeout(c.HandshakeTimeout)
}

// ResolvedHeartbeat 返回生效的心跳间隔与超时
func (c ServerConfig) ResolvedHeartbeat() (interval, timeout time.Duration) {
	return resolveServerHeartbeat(c.HeartbeatInterval, c.HeartbeatTimeout)
}

// ResolvedMaxConnLifetime 返回生效的通道最大存活时间与波动半宽
func (c ServerConfig) ResolvedMaxConnLifetime() (lifetime, jitter time.Duration) {
	return resolveMaxConnLifetime(c.MaxConnLifetime, c.MaxConnLifetimeJitter)
}

// resolveHandler 解析实际使用的 Handler
func (c ServerConfig) resolveHandler() ServerHandler {
	if c.Handler != nil {
		return c.Handler
	}
	return DefaultServerHandler{Processor: c.Processor}
}

// ClientConfig 出站 WebSocket 客户端构造参数（Handler / Processor 规则同 ServerConfig）
type ClientConfig struct {
	Name string // Name 是实例名，仅用于日志区分，可空

	CompressEnabled bool // CompressEnabled 是否启用 permessage-deflate

	ReadMaxPayloadSize  int           // ReadMaxPayloadSize 单条逻辑消息可读最大载荷；0 表示 DefaultReadMaxPayloadSize
	WriteMaxPayloadSize int           // WriteMaxPayloadSize 单条逻辑消息可写最大载荷；0 表示 DefaultWriteMaxPayloadSize
	HandshakeTimeout    time.Duration // HandshakeTimeout 握手超时（拨号+升级）；0 表示 DefaultHandshakeTimeout

	// HeartbeatInterval 主动 Ping 间隔；nil 表示 DefaultClientHeartbeatInterval；显式 0 表示不发 Ping
	HeartbeatInterval *time.Duration
	// HeartbeatTimeout 读空闲超时；nil 表示 DefaultHeartbeatTimeout；显式 0 表示不超时踢连接
	HeartbeatTimeout *time.Duration

	RequestHeader http.Header  // RequestHeader 握手额外请求头（如鉴权）
	TLSConfig     *tls.Config  // TLSConfig wss 时可选 TLS 配置；HTTPClient 非空时忽略
	HTTPClient    *http.Client // HTTPClient 自定义握手客户端（代理等）；nil 使用带超时的默认 Transport

	Handler   ClientHandler          // Handler 可选；nil 则用默认实现
	Processor ClientMessageProcessor // Processor 可选；仅 Handler 为 nil 时由默认实现消费
}

// Validate 校验 ClientConfig
func (c ClientConfig) Validate() error {
	if err := validatePayloadSize(c.WriteMaxPayloadSize, "WriteMaxPayloadSize"); err != nil {
		return err
	}
	if err := validatePayloadSize(c.ReadMaxPayloadSize, "ReadMaxPayloadSize"); err != nil {
		return err
	}
	if err := validateHeartbeat(c.HeartbeatInterval, c.HeartbeatTimeout, resolveClientHeartbeat); err != nil {
		return err
	}
	return nil
}

// ResolvedWriteMaxPayloadSize 返回生效的单条可写上限
func (c ClientConfig) ResolvedWriteMaxPayloadSize() int {
	return resolveWriteMaxPayloadSize(c.WriteMaxPayloadSize)
}

// ResolvedReadMaxPayloadSize 返回生效的单条可读上限
func (c ClientConfig) ResolvedReadMaxPayloadSize() int {
	return resolveReadMaxPayloadSize(c.ReadMaxPayloadSize)
}

// ResolvedHandshakeTimeout 返回生效的握手超时
func (c ClientConfig) ResolvedHandshakeTimeout() time.Duration {
	return resolveHandshakeTimeout(c.HandshakeTimeout)
}

// ResolvedHeartbeat 返回生效的心跳间隔与超时
func (c ClientConfig) ResolvedHeartbeat() (interval, timeout time.Duration) {
	return resolveClientHeartbeat(c.HeartbeatInterval, c.HeartbeatTimeout)
}

// resolveHandler 解析实际使用的 Handler
func (c ClientConfig) resolveHandler() ClientHandler {
	if c.Handler != nil {
		return c.Handler
	}
	return DefaultClientHandler{Processor: c.Processor}
}

// validatePayloadSize 校验单条载荷上限；0 表示使用默认值故合法
func validatePayloadSize(n int, field string) error {
	if n < 0 {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "%s 不能为负数", field)
	}
	if n > MaxWriteMaxPayloadSize {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "%s 超过 MaxWriteMaxPayloadSize", field)
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

// resolveReadMaxPayloadSize 将 0/未设置解析为 DefaultReadMaxPayloadSize
func resolveReadMaxPayloadSize(n int) int {
	if n <= 0 {
		return DefaultReadMaxPayloadSize
	}
	return n
}

// resolveHandshakeTimeout 将 0/未设置解析为 DefaultHandshakeTimeout
func resolveHandshakeTimeout(d time.Duration) time.Duration {
	if d <= 0 {
		return DefaultHandshakeTimeout
	}
	return d
}

// resolveServerHeartbeat 解析服务端心跳：nil→默认常量；显式 0 保留为关闭
func resolveServerHeartbeat(interval, timeout *time.Duration) (time.Duration, time.Duration) {
	var i, t time.Duration
	if interval == nil {
		i = DefaultServerHeartbeatInterval
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

// resolveClientHeartbeat 解析客户端心跳：nil→默认常量；显式 0 保留为关闭
func resolveClientHeartbeat(interval, timeout *time.Duration) (time.Duration, time.Duration) {
	var i, t time.Duration
	if interval == nil {
		i = DefaultClientHeartbeatInterval
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
func validateHeartbeat(interval, timeout *time.Duration, resolve func(*time.Duration, *time.Duration) (time.Duration, time.Duration)) error {
	if interval != nil && *interval < 0 {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "HeartbeatInterval 不能为负数")
	}
	if timeout != nil && *timeout < 0 {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "HeartbeatTimeout 不能为负数")
	}
	i, t := resolve(interval, timeout)
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
