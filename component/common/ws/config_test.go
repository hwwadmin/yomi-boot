// Package ws 配置校验与 Resolve 测试
//
// @author zwei
package ws

import (
	"context"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

func TestServerConfigValidateAddrRequired(t *testing.T) {
	err := ServerConfig{}.Validate()
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("空 Addr 应得 ErrInvalidArgument，实际 %v", err)
	}
}

func TestServerConfigValidateMaxConnections(t *testing.T) {
	err := ServerConfig{Addr: ":1", MaxConnections: -1}.Validate()
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("负 MaxConnections 应得 ErrInvalidArgument，实际 %v", err)
	}
}

func TestServerConfigValidateSendQueueSize(t *testing.T) {
	if err := (ServerConfig{Addr: ":1", SendQueueSize: -1}).Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("负 SendQueueSize 应得 ErrInvalidArgument，实际 %v", err)
	}
	if err := (ServerConfig{Addr: ":1", SendQueueSize: 0}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestQueueGCIntervalResolve(t *testing.T) {
	cfg := ServerConfig{Addr: ":1"}
	if got := cfg.ResolvedQueueGCInterval(); got != DefaultQueueGCInterval {
		t.Fatalf("nil => %v want %v", got, DefaultQueueGCInterval)
	}
	cfg.QueueGCInterval = Duration(0)
	if got := cfg.ResolvedQueueGCInterval(); got != 0 {
		t.Fatalf("显式 0 应关闭巡检，实际 %v", got)
	}
	if err := (ServerConfig{Addr: ":1", QueueGCInterval: Duration(-time.Second)}).Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("负 QueueGCInterval 应得 ErrInvalidArgument，实际 %v", err)
	}
}

func TestServerConfigTLSEnabledRequiresCert(t *testing.T) {
	err := ServerConfig{Addr: ":1", TLSEnabled: true}.Validate()
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("TLSEnabled 无证书应得 ErrInvalidArgument，实际 %v", err)
	}
	err = ServerConfig{Addr: ":1", TLSEnabled: true, TLSCertFile: "c.pem", TLSKeyFile: "k.pem"}.Validate()
	if err != nil {
		t.Fatal(err)
	}
}

func TestServerConfigValidatePayloadSize(t *testing.T) {
	if err := (ServerConfig{Addr: ":1", WriteMaxPayloadSize: -1}).Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("负 WriteMaxPayloadSize 应得 ErrInvalidArgument，实际 %v", err)
	}
	if err := (ServerConfig{Addr: ":1", ReadMaxPayloadSize: MaxWriteMaxPayloadSize + 1}).Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("过大 ReadMaxPayloadSize 应得 ErrInvalidArgument，实际 %v", err)
	}
}

func TestServerConfigValidateMaxFramePayloadSize(t *testing.T) {
	if err := (ServerConfig{Addr: ":1", MaxFramePayloadSize: -1}).Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("负 MaxFramePayloadSize 应得 ErrInvalidArgument，实际 %v", err)
	}
	if err := (ServerConfig{Addr: ":1", MaxFramePayloadSize: 0}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (ServerConfig{
		Addr:                ":1",
		WriteMaxPayloadSize: 1024,
		MaxFramePayloadSize: 2048,
	}).Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("MaxFrame>WriteMax 应得 ErrInvalidArgument，实际 %v", err)
	}
	if err := (ServerConfig{
		Addr:                ":1",
		WriteMaxPayloadSize: 4096,
		MaxFramePayloadSize: 1024,
	}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestServerConfigValidateHeartbeat(t *testing.T) {
	neg := Duration(-time.Second)
	if err := (ServerConfig{Addr: ":1", HeartbeatInterval: neg}).Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("负 HeartbeatInterval 应得 ErrInvalidArgument，实际 %v", err)
	}
	interval := Duration(time.Second)
	timeout := Duration(time.Second)
	if err := (ServerConfig{Addr: ":1", HeartbeatInterval: interval, HeartbeatTimeout: timeout}).Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Timeout<=Interval 应得 ErrInvalidArgument，实际 %v", err)
	}
}

func TestServerConfigValidateLifetime(t *testing.T) {
	neg := Duration(-time.Second)
	if err := (ServerConfig{Addr: ":1", MaxConnLifetime: neg}).Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("负 MaxConnLifetime 应得 ErrInvalidArgument，实际 %v", err)
	}
	life := Duration(time.Second)
	jitter := Duration(time.Second)
	if err := (ServerConfig{Addr: ":1", MaxConnLifetime: life, MaxConnLifetimeJitter: jitter}).Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Jitter>=Lifetime 应得 ErrInvalidArgument，实际 %v", err)
	}
}

func TestServerConfigResolvedDefaults(t *testing.T) {
	cfg := ServerConfig{Addr: ":1"}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := cfg.ResolvedWriteMaxPayloadSize(); got != DefaultWriteMaxPayloadSize {
		t.Fatalf("WriteMax=%d want %d", got, DefaultWriteMaxPayloadSize)
	}
	if got := cfg.ResolvedReadMaxPayloadSize(); got != DefaultReadMaxPayloadSize {
		t.Fatalf("ReadMax=%d want %d", got, DefaultReadMaxPayloadSize)
	}
	if got := cfg.ResolvedHandshakeTimeout(); got != DefaultHandshakeTimeout {
		t.Fatalf("HandshakeTimeout=%v want %v", got, DefaultHandshakeTimeout)
	}
	interval, timeout := cfg.ResolvedHeartbeat()
	if interval != DefaultServerHeartbeatInterval || timeout != DefaultHeartbeatTimeout {
		t.Fatalf("heartbeat=(%v,%v) want (%v,%v)", interval, timeout, DefaultServerHeartbeatInterval, DefaultHeartbeatTimeout)
	}
	life, jitter := cfg.ResolvedMaxConnLifetime()
	if life != DefaultMaxConnLifetime || jitter != DefaultMaxConnLifetimeJitter {
		t.Fatalf("lifetime=(%v,%v) want (%v,%v)", life, jitter, DefaultMaxConnLifetime, DefaultMaxConnLifetimeJitter)
	}
}

func TestServerConfigResolvedExplicitZero(t *testing.T) {
	cfg := ServerConfig{
		Addr:                  ":1",
		HeartbeatInterval:     Duration(0),
		HeartbeatTimeout:      Duration(0),
		MaxConnLifetime:       Duration(0),
		MaxConnLifetimeJitter: Duration(0),
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	interval, timeout := cfg.ResolvedHeartbeat()
	if interval != 0 || timeout != 0 {
		t.Fatalf("显式 0 应关闭心跳，实际 (%v,%v)", interval, timeout)
	}
	life, jitter := cfg.ResolvedMaxConnLifetime()
	if life != 0 || jitter != 0 {
		t.Fatalf("显式 0 应关闭寿命，实际 (%v,%v)", life, jitter)
	}
}

func TestClientConfigResolvedDefaults(t *testing.T) {
	cfg := ClientConfig{}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	interval, timeout := cfg.ResolvedHeartbeat()
	if interval != DefaultClientHeartbeatInterval || timeout != DefaultHeartbeatTimeout {
		t.Fatalf("heartbeat=(%v,%v) want (%v,%v)", interval, timeout, DefaultClientHeartbeatInterval, DefaultHeartbeatTimeout)
	}
}

func TestNewServerRejectsInvalid(t *testing.T) {
	if _, err := NewServer(ServerConfig{}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("NewServer 空 Addr 应得 ErrInvalidArgument，实际 %v", err)
	}
}

func TestListenAddrBeforeStart(t *testing.T) {
	srv, err := NewServer(ServerConfig{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.ListenAddr(); !errors.Is(err, ErrNotStarted) {
		t.Fatalf("未启动 ListenAddr 应得 ErrNotStarted，实际 %v", err)
	}
}

// TestResolveHandlerPropagatesName Handler 为 nil 时 Name 写入 Default*Handler
func TestResolveHandlerPropagatesName(t *testing.T) {
	sh := ServerConfig{Name: "srv-a"}.resolveHandler()
	ds, ok := sh.(DefaultServerHandler)
	if !ok || ds.Name != "srv-a" {
		t.Fatalf("Server DefaultHandler Name=%q ok=%v", ds.Name, ok)
	}
	ch := ClientConfig{Name: "cli-b"}.resolveHandler()
	dc, ok := ch.(DefaultClientHandler)
	if !ok || dc.Name != "cli-b" {
		t.Fatalf("Client DefaultHandler Name=%q ok=%v", dc.Name, ok)
	}
}

// TestWriteTimeoutValidate 负 WriteTimeout 应拒绝；0 合法（不超时）
func TestWriteTimeoutValidate(t *testing.T) {
	if err := (ServerConfig{Addr: ":1", WriteTimeout: -1}).Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("负 WriteTimeout 应得 ErrInvalidArgument，实际 %v", err)
	}
	if err := (ClientConfig{WriteTimeout: -1}).Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("负 WriteTimeout 应得 ErrInvalidArgument，实际 %v", err)
	}
	if err := (ServerConfig{Addr: ":1", WriteTimeout: 0}).Validate(); err != nil {
		t.Fatal(err)
	}
}

// TestWriteTimeoutAndQueueDefaults 验证写超时与发送队列的高可用默认值解析
func TestWriteTimeoutAndQueueDefaults(t *testing.T) {
	serverCfg := ServerConfig{Addr: ":1"}
	if err := serverCfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := serverCfg.ResolvedWriteTimeout(); got != DefaultWriteTimeout {
		t.Fatalf("Server WriteTimeout=%v want %v", got, DefaultWriteTimeout)
	}
	if got := serverCfg.ResolvedSendQueueSize(); got != DefaultSendQueueSize {
		t.Fatalf("Server SendQueueSize=%d want %d", got, DefaultSendQueueSize)
	}

	clientCfg := ClientConfig{}
	if err := clientCfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := clientCfg.ResolvedWriteTimeout(); got != DefaultWriteTimeout {
		t.Fatalf("Client WriteTimeout=%v want %v", got, DefaultWriteTimeout)
	}
	if got := clientCfg.ResolvedSendQueueSize(); got != DefaultSendQueueSize {
		t.Fatalf("Client SendQueueSize=%d want %d", got, DefaultSendQueueSize)
	}

	// 显式配置优先于默认值
	explicitServer := ServerConfig{Addr: ":1", WriteTimeout: 30 * time.Second, SendQueueSize: 1024}
	if got := explicitServer.ResolvedWriteTimeout(); got != 30*time.Second {
		t.Fatalf("显式 WriteTimeout=%v want 30s", got)
	}
	if got := explicitServer.ResolvedSendQueueSize(); got != 1024 {
		t.Fatalf("显式 SendQueueSize=%d want 1024", got)
	}
}

// TestNewServerClientApplyDefaults 验证构造器落地解析后的默认值
func TestNewServerClientApplyDefaults(t *testing.T) {
	srv, err := NewServer(ServerConfig{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	s, ok := srv.(*server)
	if !ok {
		t.Fatalf("unexpected server type %T", srv)
	}
	if s.writeTimeout != DefaultWriteTimeout || s.sendQueueSize != DefaultSendQueueSize {
		t.Fatalf("server 未落地默认值: writeTimeout=%v sendQueueSize=%d", s.writeTimeout, s.sendQueueSize)
	}
	if err := s.Close(context.Background()); err != nil {
		t.Fatalf("close server: %v", err)
	}

	cl, err := NewClient(ClientConfig{})
	if err != nil {
		t.Fatal(err)
	}
	c, ok := cl.(*client)
	if !ok {
		t.Fatalf("unexpected client type %T", cl)
	}
	if c.writeTimeout != DefaultWriteTimeout || c.sendQueueSize != DefaultSendQueueSize {
		t.Fatalf("client 未落地默认值: writeTimeout=%v sendQueueSize=%d", c.writeTimeout, c.sendQueueSize)
	}
	_ = c.Close(CloseNormalClosure, "test done")
}
