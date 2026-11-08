package logger

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("write") }

type fnWriter func([]byte) (int, error)

func (f fnWriter) Write(p []byte) (int, error) { return f(p) }

func TestCompressOrDefault_ExplicitFalse(t *testing.T) {
	off := false
	if (RotationConfig{Compress: &off}).CompressOrDefault() {
		t.Fatal("显式 false 不应按默认压缩")
	}
}

func TestLoggerConfigMustInitAndAsyncEdges(t *testing.T) {
	var cfg *Config
	if err := cfg.Normalize(); err == nil {
		t.Fatal("nil 配置应失败")
	}
	negative := Config{Rotation: testRotation(), AsyncQueueSize: -1}
	if err := negative.Normalize(); err == nil {
		t.Fatal("负数队列应失败")
	}
	blankKey := Config{Rotation: testRotation(), Fields: map[string]string{" ": "x"}}
	if err := blankKey.Normalize(); err == nil {
		t.Fatal("空白字段名应失败")
	}
	noTotal := testRotation()
	noTotal.MaxTotalMB = 0
	if err := (&Config{Rotation: noTotal}).Normalize(); err == nil {
		t.Fatal("总容量必须大于 0")
	}
	if !(RotationConfig{}).CompressOrDefault() || !(AccessConfig{}).EnabledOrDefault() {
		t.Fatal("压缩和 access 默认应开启")
	}
	off := false
	if (Config{ConsoleEnabled: &off}).ConsoleEnabledOrDefault() || (Config{AsyncEnabled: &off}).AsyncEnabledOrDefault() {
		t.Fatal("显式关闭的开关应为 false")
	}

	func() {
		defer func() { _ = recover() }()
		MustInit("svc", "test", Config{}, nil)
		t.Fatal("非法配置应 panic")
	}()

	prev := defaultHub.Load()
	t.Cleanup(func() { defaultHub.Store(prev) })
	dir := t.TempDir()
	closeFn := MustInit("svc", "test", Config{
		Dir:          dir,
		Level:        "debug",
		AsyncEnabled: &off,
		Rotation:     testRotation(),
		Access:       AccessConfig{ToConsole: true, RecordBody: true},
	}, nil)
	Debug(context.Background(), "debug-file")
	Info(context.Background(), "info-file")
	Warn(context.Background(), "warn-file")
	Error(context.Background(), "error-file")
	Access(context.Background(), "access-file")
	if err := Flush(time.Second); err != nil {
		t.Fatal(err)
	}
	if CurrentAccessConfig().RecordBody != true {
		t.Fatal("应读到当前 access 配置")
	}
	_ = MaskQuery("token=1")
	closeFn()

	if err := (&logHub{}).Flush(-1); err != nil {
		t.Fatal(err)
	}
	block := make(chan struct{})
	slow := newAsyncSink("slow", fnWriter(func([]byte) (int, error) {
		<-block
		return 1, nil
	}), 0)
	fast := newAsyncSink("fast", &bytes.Buffer{}, 1)
	slow.Write([]byte("x"))
	time.Sleep(20 * time.Millisecond)
	flushErr := (&logHub{sinks: []*asyncSink{slow, fast}}).Flush(time.Nanosecond)
	close(block)
	slow.Stop()
	fast.Stop()
	if flushErr == nil {
		t.Fatal("阻塞的 sink 应导致刷新失败")
	}

	stopped := newAsyncSink("stopped", &bytes.Buffer{}, 1)
	stopped.Stop()
	if _, err := stopped.Write([]byte("after")); err != nil {
		t.Fatal(err)
	}
	stopped.warnOut = nil
	stopped.dropped.Store(2)
	if !stopped.emitIfAny() {
		t.Fatal("有丢弃次数时应发出告警")
	}
	stopped.dropped.Store(2)
	stopped.warnOut = failWriter{}
	if stopped.emitIfAny() {
		t.Fatal("告警写入失败应把次数加回去")
	}
}
