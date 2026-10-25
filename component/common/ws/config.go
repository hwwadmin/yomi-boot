// Package ws 定义 Server/Client 构造配置
//
// @author zwei
package ws

import (
	"crypto/tls"
	"net/http"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

const (
	// DefaultWriteMaxPayloadSize 单条逻辑消息默认可写上限（256KiB）
	DefaultWriteMaxPayloadSize = 256 * 1024

	// DefaultReadMaxPayloadSize 单条逻辑消息默认可读上限（256KiB）
	DefaultReadMaxPayloadSize = DefaultWriteMaxPayloadSize

	// MaxWriteMaxPayloadSize 允许配置的单条可写/可读上限（8MiB）
	MaxWriteMaxPayloadSize = 8 * 1024 * 1024

	// DefaultServerHeartbeatInterval 服务端默认心跳 Ping 间隔
	DefaultServerHeartbeatInterval = 10 * time.Second

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

	// DefaultWriteTimeout 单条消息默认写出超时；超时强制拆线，防止慢客户端卡死写循环
	DefaultWriteTimeout = 10 * time.Second

	// DefaultSendQueueSize 每连接发送队列默认容量（条数）；有界队列防慢客户端撑爆内存
	DefaultSendQueueSize = 256

	// DefaultQueueGCInterval 默认孤儿发送队列巡检间隔
	DefaultQueueGCInterval = 30 * time.Second
)

// Duration 返回 d 的指针，用于需区分 nil（默认）与显式 0（关闭）的配置项
func Duration(d time.Duration) *time.Duration {
	return &d
}

// ServerConfig 独立端口 WebSocket 服务构造参数；由业务填充，本包不读配置文件。
//
// Handler / Processor：
//   - Handler == nil：用 DefaultServerHandler，OnPayload 消费 Processor
//   - Handler != nil：用自定义 Handler，Processor 不自动接入
type ServerConfig struct {
	// Name 实例名，写入默认 Handler 日志
	// 默认：空
	Name string
	// Addr 监听地址，必填，如 ":7101"
	Addr string

	// CompressEnabled 是否启用 permessage-deflate 压缩
	// 默认：false（关闭）
	CompressEnabled bool

	// ReadMaxPayloadSize 单条逻辑消息可读上限（字节）
	// 默认：0 → DefaultReadMaxPayloadSize（256KiB）
	ReadMaxPayloadSize int
	// WriteMaxPayloadSize 单条逻辑消息可写上限（字节）
	// 默认：0 → DefaultWriteMaxPayloadSize（256KiB）
	WriteMaxPayloadSize int
	// WriteTimeout 单条消息写出超时（整条含分片），超时强制拆线
	// 默认：0 → DefaultWriteTimeout（10s）
	WriteTimeout time.Duration
	// HandshakeTimeout 握手超时
	// 默认：0 → DefaultHandshakeTimeout（5s）
	HandshakeTimeout time.Duration

	// MaxFramePayloadSize 出站协议分片目标帧载荷（字节）；超过则 RFC continuation 分片
	// 默认：0（关闭分片）；须 ≤ 生效的 WriteMaxPayloadSize；压缩开启时帧长尽力而为
	MaxFramePayloadSize int

	// HeartbeatInterval 主动 Ping 间隔
	// 默认：nil → DefaultServerHeartbeatInterval（10s）；显式 0 表示不发 Ping
	HeartbeatInterval *time.Duration
	// HeartbeatTimeout 读空闲超时，超时踢连接
	// 默认：nil → DefaultHeartbeatTimeout（30s）；显式 0 表示不超时踢
	HeartbeatTimeout *time.Duration

	// MaxConnLifetime 单连接最大存活时间（自握手起，流量不续命）
	// 默认：nil → DefaultMaxConnLifetime（24h）；显式 0 表示不限制
	MaxConnLifetime *time.Duration
	// MaxConnLifetimeJitter 存活时间抖动半宽，用于打散到期
	// 默认：nil → DefaultMaxConnLifetimeJitter（30s）；显式 0 表示不抖动
	MaxConnLifetimeJitter *time.Duration

	// MaxConnections 同时在线连接上限；超限握手后 CloseTryAgainLater
	// 默认：0（不限制）
	MaxConnections int

	// SendQueueSize 每连接发送队列容量（条数）；满则阻塞入队
	// 默认：0 → DefaultSendQueueSize（256）；写超时保证写循环限期排空或拆线，有界队列封顶单连接内存
	SendQueueSize int
	// QueueGCInterval 孤儿发送队列巡检间隔
	// 默认：nil → DefaultQueueGCInterval（30s）；显式 0 表示关闭巡检
	QueueGCInterval *time.Duration

	// OriginPatterns 允许的 Origin host 模式（path.Match）
	// 默认：空（不做 Origin 校验）
	OriginPatterns []string

	// TLSEnabled 是否 TLS 监听；为 true 时须配证书与私钥
	// 默认：false
	TLSEnabled bool
	// TLSCertFile TLS 证书路径；TLSEnabled 时必填
	// 默认：空
	TLSCertFile string
	// TLSKeyFile TLS 私钥路径；TLSEnabled 时必填
	// 默认：空
	TLSKeyFile string
	// TLSConfig 可选 TLS 配置，与证书文件合并（Certificates 由文件覆盖）
	// 默认：nil
	TLSConfig *tls.Config

	// Authorize 握手鉴权；返回 false 拒绝
	// 默认：nil（放行）
	Authorize func(r *http.Request) bool

	// Handler 会话回调；自定义时 Processor 不自动接入
	// 默认：nil → DefaultServerHandler
	Handler ServerHandler
	// Processor 业务消费 OnPayload；仅 Handler 为 nil 时由默认 Handler 调用
	// 默认：nil
	Processor ServerMessageProcessor
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
	if c.WriteTimeout < 0 {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "WriteTimeout 不能为负数")
	}
	if err := validateMaxFramePayloadSize(c.MaxFramePayloadSize, c.ResolvedWriteMaxPayloadSize()); err != nil {
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
	if c.SendQueueSize < 0 {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "SendQueueSize 不能为负数")
	}
	if err := validateQueueGCInterval(c.QueueGCInterval); err != nil {
		return err
	}
	if c.TLSEnabled {
		if c.TLSCertFile == "" || c.TLSKeyFile == "" {
			return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "TLSEnabled 时 TLSCertFile 与 TLSKeyFile 不能为空")
		}
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

// ResolvedWriteTimeout 返回生效的单条消息写出超时
func (c ServerConfig) ResolvedWriteTimeout() time.Duration {
	return resolveWriteTimeout(c.WriteTimeout)
}

// ResolvedSendQueueSize 返回生效的发送队列容量
func (c ServerConfig) ResolvedSendQueueSize() int {
	return resolveSendQueueSize(c.SendQueueSize)
}

// ResolvedHeartbeat 返回生效的心跳间隔与超时
func (c ServerConfig) ResolvedHeartbeat() (interval, timeout time.Duration) {
	return resolveServerHeartbeat(c.HeartbeatInterval, c.HeartbeatTimeout)
}

// ResolvedMaxConnLifetime 返回生效的通道最大存活时间与波动半宽
func (c ServerConfig) ResolvedMaxConnLifetime() (lifetime, jitter time.Duration) {
	return resolveMaxConnLifetime(c.MaxConnLifetime, c.MaxConnLifetimeJitter)
}

// ResolvedQueueGCInterval 返回生效的队列巡检间隔；0 表示关闭巡检
func (c ServerConfig) ResolvedQueueGCInterval() time.Duration {
	return resolveQueueGCInterval(c.QueueGCInterval)
}

// resolveHandler 解析实际使用的 Handler
func (c ServerConfig) resolveHandler() ServerHandler {
	if c.Handler != nil {
		return c.Handler
	}
	return DefaultServerHandler{Name: c.Name, Processor: c.Processor}
}

// ClientConfig 出站 WebSocket 客户端构造参数（Handler / Processor 规则同 ServerConfig）
type ClientConfig struct {
	// Name 实例名，写入默认 Handler 日志
	// 默认：空
	Name string

	// CompressEnabled 是否启用 permessage-deflate 压缩
	// 默认：false（关闭）
	CompressEnabled bool

	// ReadMaxPayloadSize 单条逻辑消息可读上限（字节）
	// 默认：0 → DefaultReadMaxPayloadSize（256KiB）
	ReadMaxPayloadSize int
	// WriteMaxPayloadSize 单条逻辑消息可写上限（字节）
	// 默认：0 → DefaultWriteMaxPayloadSize（256KiB）
	WriteMaxPayloadSize int
	// WriteTimeout 单条消息写出超时（整条含分片），超时强制拆线
	// 默认：0 → DefaultWriteTimeout（10s）
	WriteTimeout time.Duration
	// HandshakeTimeout 拨号与升级握手超时
	// 默认：0 → DefaultHandshakeTimeout（5s）
	HandshakeTimeout time.Duration

	// MaxFramePayloadSize 出站协议分片目标帧载荷（字节）；超过则 RFC continuation 分片
	// 默认：0（关闭分片）；须 ≤ 生效的 WriteMaxPayloadSize；压缩开启时帧长尽力而为
	MaxFramePayloadSize int

	// HeartbeatInterval 主动 Ping 间隔
	// 默认：nil → DefaultClientHeartbeatInterval（10s）；显式 0 表示不发 Ping
	HeartbeatInterval *time.Duration
	// HeartbeatTimeout 读空闲超时，超时踢连接
	// 默认：nil → DefaultHeartbeatTimeout（30s）；显式 0 表示不超时踢
	HeartbeatTimeout *time.Duration

	// SendQueueSize 发送队列容量（条数）；满则阻塞入队
	// 默认：0 → DefaultSendQueueSize（256）
	SendQueueSize int
	// QueueGCInterval 孤儿发送队列巡检间隔
	// 默认：nil → DefaultQueueGCInterval（30s）；显式 0 表示关闭巡检
	QueueGCInterval *time.Duration

	// RequestHeader 握手额外请求头（如鉴权）
	// 默认：nil
	RequestHeader http.Header
	// TLSConfig wss 可选 TLS 配置；HTTPClient 非空时忽略本字段
	// 默认：nil
	TLSConfig *tls.Config
	// HTTPClient 自定义握手客户端（代理等）
	// 默认：nil → 带握手超时的默认 Transport
	HTTPClient *http.Client

	// Handler 会话回调；自定义时 Processor 不自动接入
	// 默认：nil → DefaultClientHandler
	Handler ClientHandler
	// Processor 业务消费 OnPayload；仅 Handler 为 nil 时由默认 Handler 调用
	// 默认：nil
	Processor ClientMessageProcessor
}

// Validate 校验 ClientConfig
func (c ClientConfig) Validate() error {
	if err := validatePayloadSize(c.WriteMaxPayloadSize, "WriteMaxPayloadSize"); err != nil {
		return err
	}
	if err := validatePayloadSize(c.ReadMaxPayloadSize, "ReadMaxPayloadSize"); err != nil {
		return err
	}
	if c.WriteTimeout < 0 {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "WriteTimeout 不能为负数")
	}
	if err := validateMaxFramePayloadSize(c.MaxFramePayloadSize, c.ResolvedWriteMaxPayloadSize()); err != nil {
		return err
	}
	if err := validateHeartbeat(c.HeartbeatInterval, c.HeartbeatTimeout, resolveClientHeartbeat); err != nil {
		return err
	}
	if c.SendQueueSize < 0 {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "SendQueueSize 不能为负数")
	}
	if err := validateQueueGCInterval(c.QueueGCInterval); err != nil {
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

// ResolvedWriteTimeout 返回生效的单条消息写出超时
func (c ClientConfig) ResolvedWriteTimeout() time.Duration {
	return resolveWriteTimeout(c.WriteTimeout)
}

// ResolvedSendQueueSize 返回生效的发送队列容量
func (c ClientConfig) ResolvedSendQueueSize() int {
	return resolveSendQueueSize(c.SendQueueSize)
}

// ResolvedHeartbeat 返回生效的心跳间隔与超时
func (c ClientConfig) ResolvedHeartbeat() (interval, timeout time.Duration) {
	return resolveClientHeartbeat(c.HeartbeatInterval, c.HeartbeatTimeout)
}

// ResolvedQueueGCInterval 返回生效的队列巡检间隔；0 表示关闭巡检
func (c ClientConfig) ResolvedQueueGCInterval() time.Duration {
	return resolveQueueGCInterval(c.QueueGCInterval)
}

// resolveHandler 解析实际使用的 Handler
func (c ClientConfig) resolveHandler() ClientHandler {
	if c.Handler != nil {
		return c.Handler
	}
	return DefaultClientHandler{Name: c.Name, Processor: c.Processor}
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

// validateMaxFramePayloadSize 校验分片目标帧长；0 表示关闭；>0 时不得超过生效的 WriteMax
func validateMaxFramePayloadSize(maxFrame, writeMax int) error {
	if maxFrame < 0 {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "MaxFramePayloadSize 不能为负数")
	}
	if maxFrame > 0 && maxFrame > writeMax {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "MaxFramePayloadSize 不能大于 WriteMaxPayloadSize")
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

// resolveWriteTimeout 将 0/未设置解析为 DefaultWriteTimeout
func resolveWriteTimeout(d time.Duration) time.Duration {
	if d <= 0 {
		return DefaultWriteTimeout
	}
	return d
}

// resolveSendQueueSize 将 0/未设置解析为 DefaultSendQueueSize
func resolveSendQueueSize(n int) int {
	if n <= 0 {
		return DefaultSendQueueSize
	}
	return n
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

// resolveQueueGCInterval 解析队列巡检：nil→默认；显式 0 表示关闭
func resolveQueueGCInterval(d *time.Duration) time.Duration {
	if d == nil {
		return DefaultQueueGCInterval
	}
	return *d
}

// validateQueueGCInterval 校验巡检间隔不能为负
func validateQueueGCInterval(d *time.Duration) error {
	if d != nil && *d < 0 {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "QueueGCInterval 不能为负数")
	}
	return nil
}
