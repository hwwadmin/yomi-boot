// 异步写入层与控制台配置语义单元测试
//
// @author zwei

package logger

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// gateWriter 首次 Write 阻塞直至 release 关闭，用于把消费协程钉在写入点
type gateWriter struct {
	mu       sync.Mutex
	buf      bytes.Buffer
	release  chan struct{}
	entered  chan struct{}
	released atomic.Bool
}

// newGateWriter 创建阻塞写入器
func newGateWriter() *gateWriter {
	return &gateWriter{release: make(chan struct{}), entered: make(chan struct{})}
}

// Write 首次调用阻塞在 release；之后的调用直接落 buf
func (w *gateWriter) Write(p []byte) (int, error) {
	if w.released.CompareAndSwap(false, true) {
		close(w.entered)
		<-w.release
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(p)
	return len(p), nil
}

// snapshot 返回已落盘内容
func (w *gateWriter) snapshot() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// panicOnceWriter 首次 Write panic，其后正常落盘
type panicOnceWriter struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	armed bool
}

// Write 首次调用 panic，之后写入 buf
func (w *panicOnceWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.armed {
		w.armed = false
		panic("sink write boom")
	}
	w.buf.Write(p)
	return len(p), nil
}

// snapshot 返回已落盘内容
func (w *panicOnceWriter) snapshot() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// TestAsyncSink_FIFOOrderAndFlush 验证同 sink 内按入队顺序落盘，Flush 等待全部落盘
func TestAsyncSink_FIFOOrderAndFlush(t *testing.T) {
	dst := &bytes.Buffer{}
	s := newAsyncSink("test", dst, 64)

	for _, line := range []string{"a", "b", "c"} {
		if _, err := s.Write([]byte(line)); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if err := s.Flush(time.Second); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if got := dst.String(); got != "abc" {
		t.Fatalf("期望按序落盘 abc，实际 %q", got)
	}
	s.Stop()
}

// TestAsyncSink_DropsWhenFull 验证队列满时丢弃且不阻塞调用方
func TestAsyncSink_DropsWhenFull(t *testing.T) {
	dst := newGateWriter()
	s := newAsyncSink("test", dst, 1)

	// 占住消费协程：首条进入 dst.Write 并阻塞，此时队列为空
	if _, err := s.Write([]byte("first")); err != nil {
		t.Fatalf("write first: %v", err)
	}
	select {
	case <-dst.entered:
	case <-time.After(time.Second):
		t.Fatal("消费协程未进入写入点")
	}

	if _, err := s.Write([]byte("A")); err != nil { // 入队
		t.Fatalf("write A: %v", err)
	}
	if _, err := s.Write([]byte("B")); err != nil { // 队列满，丢弃
		t.Fatalf("write B: %v", err)
	}
	if got := s.dropped.Load(); got != 1 {
		t.Fatalf("期望丢弃 1 条，实际 %d", got)
	}

	close(dst.release)
	if err := s.Flush(time.Second); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if got := dst.snapshot(); got != "firstA" {
		t.Fatalf("期望落盘 firstA（B 被丢弃），实际 %q", got)
	}
	s.Stop()
}

// TestAsyncSink_StopDrainsAndWriteFallback 验证 Stop 排空残余，停后写入走同步兜底
func TestAsyncSink_StopDrainsAndWriteFallback(t *testing.T) {
	dst := &bytes.Buffer{}
	s := newAsyncSink("test", dst, 64)

	for i := 0; i < 10; i++ {
		if _, err := s.Write([]byte("x")); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	s.Stop()

	if got := dst.String(); got != strings.Repeat("x", 10) {
		t.Fatalf("Stop 应排空队列，实际 %q", got)
	}
	// 停止后写入：同步直写底层
	if _, err := s.Write([]byte("after")); err != nil {
		t.Fatalf("write after stop: %v", err)
	}
	if got := dst.String(); !strings.HasSuffix(got, "after") {
		t.Fatalf("停后写入应同步落盘，实际 %q", got)
	}
}

// TestAsyncSink_FlushTimeout 验证落盘卡死时 Flush 按超时返回错误
func TestAsyncSink_FlushTimeout(t *testing.T) {
	dst := newGateWriter()
	s := newAsyncSink("test", dst, 64)

	if _, err := s.Write([]byte("stuck")); err != nil {
		t.Fatalf("write: %v", err)
	}
	select {
	case <-dst.entered:
	case <-time.After(time.Second):
		t.Fatal("消费协程未进入写入点")
	}

	if err := s.Flush(50 * time.Millisecond); err == nil {
		t.Fatal("落盘卡死时 Flush 应超时报错")
	}
	close(dst.release)
	if err := s.Flush(time.Second); err != nil {
		t.Fatalf("放行后 Flush 应成功: %v", err)
	}
	s.Stop()
}

// TestAsyncSink_PanicRecovers 验证底层写入 panic 后 sink 自愈继续工作
func TestAsyncSink_PanicRecovers(t *testing.T) {
	dst := &panicOnceWriter{armed: true}
	s := newAsyncSink("test", dst, 64)

	if _, err := s.Write([]byte("boom")); err != nil {
		t.Fatalf("write: %v", err)
	}
	// 首条触发 panic，消费协程退避重建后处理屏障
	if err := s.Flush(2 * time.Second); err != nil {
		t.Fatalf("panic 后 Flush 应自愈成功: %v", err)
	}
	if _, err := s.Write([]byte("ok")); err != nil {
		t.Fatalf("write ok: %v", err)
	}
	if err := s.Flush(time.Second); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if got := dst.snapshot(); got != "ok" {
		t.Fatalf("panic 条目不应落盘、后续应正常，实际 %q", got)
	}
	s.Stop()
}

// TestLogger_ConsoleDisabled 验证 console_enabled=false 不创建控制台 sink，文件照写
func TestLogger_ConsoleDisabled(t *testing.T) {
	dir := t.TempDir()
	consoleOff := false
	cfg := Config{Dir: dir, Level: "info", ConsoleEnabled: &consoleOff, Rotation: testRotation()}
	closeFn, err := InitLogger("test-svc", "test", cfg, nil)
	if err != nil {
		t.Fatalf("InitLogger: %v", err)
	}
	defer func() { _ = closeFn() }()

	Info(context.Background(), "file only line")
	_ = Sync()

	assertFileContains(t, filepath.Join(dir, "info", "info.log"), "file only line")
	for _, s := range defaultHub.Load().sinks {
		if s.name == "console" {
			t.Fatal("console_enabled=false 不应创建 console sink")
		}
	}
}

// TestLogger_AsyncDisabledSyncPath 验证 async_enabled=false 退化为同步直写
func TestLogger_AsyncDisabledSyncPath(t *testing.T) {
	dir := t.TempDir()
	asyncOff := false
	cfg := Config{Dir: dir, Level: "info", AsyncEnabled: &asyncOff, Rotation: testRotation()}
	closeFn, err := InitLogger("test-svc", "test", cfg, nil)
	if err != nil {
		t.Fatalf("InitLogger: %v", err)
	}
	defer func() { _ = closeFn() }()

	Info(context.Background(), "sync path line")
	_ = Sync()

	if sinks := defaultHub.Load().sinks; len(sinks) != 0 {
		t.Fatalf("async_enabled=false 不应创建 sink，实际 %d 个", len(sinks))
	}
	assertFileContains(t, filepath.Join(dir, "info", "info.log"), "sync path line")
}

// TestLogger_ConsoleLevelFollowsLevel 验证控制台等级跟随 level（level=warn 时 Info 不上控制台）
func TestLogger_ConsoleLevelFollowsLevel(t *testing.T) {
	dir := t.TempDir()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	oldStdout := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = oldStdout })

	cfg := Config{Dir: dir, Level: "warn", Rotation: testRotation()}
	closeFn, err := InitLogger("test-svc", "test", cfg, nil)
	if err != nil {
		t.Fatalf("InitLogger: %v", err)
	}
	defer func() { _ = closeFn() }()

	ctx := context.Background()
	Info(ctx, "info-line-not-on-console")
	Warn(ctx, "warn-line-on-console")
	_ = Sync()
	_ = w.Close()

	out := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		out <- string(b)
	}()
	console := <-out
	_ = r.Close()

	if strings.Contains(console, "info-line-not-on-console") {
		t.Fatalf("level=warn 时 Info 不应上控制台: %q", console)
	}
	if !strings.Contains(console, "warn-line-on-console") {
		t.Fatalf("Warn 应上控制台: %q", console)
	}
}
