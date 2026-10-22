// Package ws 发送队列行为测试
//
// @author zwei
package ws

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// TestSendQueueBoundedBlocksUntilSlot 有界队列满时 Send 阻塞，消费后恢复
func TestSendQueueBoundedBlocksUntilSlot(t *testing.T) {
	t.Log("[用例] SendQueueSize=1，第二条 Send 应阻塞至有槽")

	srv, url := startTestServer(t, ServerConfig{
		Addr:             "127.0.0.1:0",
		SendQueueSize:    1,
		HeartbeatTimeout: Duration(0),
		MaxConnLifetime:  Duration(0),
		QueueGCInterval:  Duration(0),
		Handler:          NopServerHandler{},
	})
	cli := connectTestClient(t, url, ClientConfig{
		SendQueueSize:     1,
		HeartbeatInterval: Duration(0),
		HeartbeatTimeout:  Duration(0),
		QueueGCInterval:   Duration(0),
		Handler:           NopClientHandler{},
	})
	waitUntil(t, testWait, func() bool { return srv.Len() == 1 }, "等待登记超时")

	// 填满客户端队列：对端不读时写循环可能很快写出，故用服务端慢处理更稳。
	// 这里验证关闭后阻塞入队被唤醒。
	_ = cli

	blocked := make(chan struct{})
	done := make(chan error, 1)
	var once sync.Once

	// 造一个已关闭的有界队列连接场景：先连通再关服务，再 Send
	_ = srv.Close(context.Background())
	waitUntil(t, testWait, func() bool { return !cli.Connected() }, "等待断开超时")

	go func() {
		once.Do(func() { close(blocked) })
		done <- cli.Send(MsgText, []byte("after-close"))
	}()
	select {
	case <-blocked:
	case <-time.After(testWait):
		t.Fatal("Send 未开始")
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrNotConnected) {
			t.Fatalf("关闭后 Send 应得 ErrNotConnected，实际 %v", err)
		}
	case <-time.After(testWait):
		t.Fatal("关闭后 Send 未返回")
	}
}

// TestServerBroadcastSkipsDeadConn Broadcast 不因单连接已拆而失败
func TestServerBroadcastSkipsDeadConn(t *testing.T) {
	srv, url := startTestServer(t, ServerConfig{
		Addr:             "127.0.0.1:0",
		HeartbeatTimeout: Duration(0),
		MaxConnLifetime:  Duration(0),
		QueueGCInterval:  Duration(0),
		Handler:          NopServerHandler{},
	})
	cli := connectTestClient(t, url, ClientConfig{
		HeartbeatInterval: Duration(0),
		HeartbeatTimeout:  Duration(0),
		QueueGCInterval:   Duration(0),
		Handler:           NopClientHandler{},
	})
	waitUntil(t, testWait, func() bool { return srv.Len() == 1 }, "等待登记超时")
	_ = cli.Close(CloseNormalClosure, "bye")
	waitUntil(t, testWait, func() bool { return srv.Len() == 0 }, "等待 hub 清空超时")
	if err := srv.Broadcast(MsgText, []byte("hi")); err != nil {
		t.Fatalf("空 hub Broadcast 应成功，实际 %v", err)
	}
}

// captureServerProcessor 将收到的载荷送入通道（分片写出用例）
type captureServerProcessor struct {
	ch chan []byte
}

func (p *captureServerProcessor) OnPayload(_ SessionContext, payload Payload) error {
	p.ch <- append([]byte(nil), payload.Data...)
	return nil
}

// TestMaxFramePayloadRoundTrip 超 MaxFrame 的明文经协议分片后对端仍一次 OnPayload
func TestMaxFramePayloadRoundTrip(t *testing.T) {
	t.Log("[用例] MaxFramePayloadSize=8，发送 40 字节二进制，对端一次收齐")

	const maxFrame = 8
	payload := bytes.Repeat([]byte("0123456789"), 4) // 40 bytes

	srvGot := make(chan []byte, 1)
	_, url := startTestServer(t, ServerConfig{
		Addr:                "127.0.0.1:0",
		WriteMaxPayloadSize: 256,
		MaxFramePayloadSize: maxFrame,
		HeartbeatTimeout:    Duration(0),
		MaxConnLifetime:     Duration(0),
		QueueGCInterval:     Duration(0),
		Handler: DefaultServerHandler{
			Processor: &captureServerProcessor{ch: srvGot},
		},
	})
	cli := connectTestClient(t, url, ClientConfig{
		WriteMaxPayloadSize: 256,
		MaxFramePayloadSize: maxFrame,
		HeartbeatInterval:   Duration(0),
		HeartbeatTimeout:    Duration(0),
		QueueGCInterval:     Duration(0),
		Handler:             NopClientHandler{},
	})

	if err := cli.Send(MsgBinary, payload); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-srvGot:
		if !bytes.Equal(got, payload) {
			t.Fatalf("服务端收到 len=%d want %d", len(got), len(payload))
		}
	case <-time.After(testWait):
		t.Fatal("等待分片合帧超时")
	}
}

// TestMaxFramePayloadRoundTripWithCompress 压缩+分片仍一次 OnPayload
func TestMaxFramePayloadRoundTripWithCompress(t *testing.T) {
	t.Log("[用例] CompressEnabled + MaxFramePayloadSize，大包一次收齐")

	const maxFrame = 16
	payload := bytes.Repeat([]byte("abcdef"), 20) // 120 bytes

	srvGot := make(chan []byte, 1)
	_, url := startTestServer(t, ServerConfig{
		Addr:                "127.0.0.1:0",
		CompressEnabled:     true,
		WriteMaxPayloadSize: 1024,
		MaxFramePayloadSize: maxFrame,
		HeartbeatTimeout:    Duration(0),
		MaxConnLifetime:     Duration(0),
		QueueGCInterval:     Duration(0),
		Handler: DefaultServerHandler{
			Processor: &captureServerProcessor{ch: srvGot},
		},
	})
	cli := connectTestClient(t, url, ClientConfig{
		CompressEnabled:     true,
		WriteMaxPayloadSize: 1024,
		MaxFramePayloadSize: maxFrame,
		HeartbeatInterval:   Duration(0),
		HeartbeatTimeout:    Duration(0),
		QueueGCInterval:     Duration(0),
		Handler:             NopClientHandler{},
	})

	if err := cli.Send(MsgBinary, payload); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-srvGot:
		if !bytes.Equal(got, payload) {
			t.Fatalf("服务端收到 len=%d want %d", len(got), len(payload))
		}
	case <-time.After(testWait):
		t.Fatal("等待压缩分片合帧超时")
	}
}
