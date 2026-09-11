// Package ws 配置校验与 Resolve 测试
//
// @author zwei
package ws

import (
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"
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
