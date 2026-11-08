package logger

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) {
	return 0, errors.New("write")
}

type fnWriter func([]byte) (int, error)

func (f fnWriter) Write(p []byte) (int, error) { return f(p) }

type errReadCloser struct{}

func (errReadCloser) Read([]byte) (int, error) { return 0, errors.New("read") }
func (errReadCloser) Close() error             { return nil }

type logArray struct{}

func (logArray) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	enc.AppendString("a")
	return nil
}

type logObject struct{}

func (logObject) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("k", "v")
	return nil
}

func TestLoggerAPIAndEncoderEdges(t *testing.T) {
	prev := defaultHub.Load()
	t.Cleanup(func() { defaultHub.Store(prev) })
	defaultHub.Store(nil)

	Debug(context.Background(), "debug")
	DebugWithoutCtx("debug")
	InfoWithoutCtx("info", 7, "n")
	WarnWithoutCtx("warn", "only-key")
	ErrorWithoutCtx("error")
	Access(context.Background(), "access")
	write(context.Background(), zapcore.DPanicLevel, "unexpected")
	if dropTraceparent(nil) != nil || keepCallerTraceparent(nil) != nil || kvToFields() != nil {
		t.Fatal("空字段应返回 nil")
	}
	if accessLogger() == nil || infoLogger() == nil || errorLogger() == nil || debugLogger() == nil || fallbackLogger() == nil {
		t.Fatal("未初始化时应回退到开发日志")
	}
	if Flush(0) != nil || Sync() != nil {
		t.Fatal("未初始化时刷新应为空操作")
	}
	_ = CurrentAccessConfig()
	if MaskQuery("a=1") != "a=1" {
		t.Fatal("未初始化时查询串应原样返回")
	}

	oldOut, oldErr := gin.DefaultWriter, gin.DefaultErrorWriter
	t.Cleanup(func() {
		gin.DefaultWriter = oldOut
		gin.DefaultErrorWriter = oldErr
	})
	SetupGin()
	if _, err := gin.DefaultWriter.Write([]byte(" \n")); err != nil {
		t.Fatal(err)
	}
	if _, err := gin.DefaultWriter.Write([]byte("gin debug")); err != nil {
		t.Fatal(err)
	}
	if _, err := gin.DefaultErrorWriter.Write([]byte("gin warn")); err != nil {
		t.Fatal(err)
	}

	enc := newTracingConsoleEncoder(zap.NewDevelopmentEncoderConfig()).(*tracingConsoleEncoder)
	enc.AddArray("arr", logArray{})
	enc.AddObject("obj", logObject{})
	enc.AddBinary("bin", []byte{1})
	enc.AddByteString("bs", []byte("b"))
	enc.AddBool("ok", true)
	enc.AddComplex128("c128", 1+2i)
	enc.AddComplex64("c64", 1+2i)
	enc.AddDuration("d", time.Second)
	enc.AddFloat64("f64", 1.5)
	enc.AddFloat32("f32", 1.5)
	enc.AddInt("i", 1)
	enc.AddInt64("i64", 1)
	enc.AddInt32("i32", 1)
	enc.AddInt16("i16", 1)
	enc.AddInt8("i8", 1)
	enc.AddString("s", "v")
	enc.AddTime("t", time.Unix(0, 0))
	enc.AddUint("u", 1)
	enc.AddUint64("u64", 1)
	enc.AddUint32("u32", 1)
	enc.AddUint16("u16", 1)
	enc.AddUint8("u8", 1)
	enc.AddUintptr("p", 1)
	if err := enc.AddReflected("r", map[string]string{"a": "b"}); err != nil {
		t.Fatal(err)
	}
	enc.OpenNamespace("ns")
	cloned := enc.Clone()
	buf, err := cloned.EncodeEntry(zapcore.Entry{Message: "m", Time: time.Now()}, []zapcore.Field{zap.String("k", "v")})
	if err != nil {
		t.Fatal(err)
	}
	buf.Free()

	arr := &stringArrayEncoder{}
	arr.AppendBool(true)
	arr.AppendByteString([]byte("x"))
	arr.AppendComplex128(0)
	arr.AppendComplex64(0)
	arr.AppendFloat64(0)
	arr.AppendFloat32(0)
	arr.AppendInt(0)
	arr.AppendInt64(0)
	arr.AppendInt32(0)
	arr.AppendInt16(0)
	arr.AppendInt8(0)
	arr.AppendString("s")
	arr.AppendUint(0)
	arr.AppendUint64(0)
	arr.AppendUint32(0)
	arr.AppendUint16(0)
	arr.AppendUint8(0)
	arr.AppendUintptr(0)
	if arr.String() == "" {
		t.Fatal("字符串编码器应保留追加的文本")
	}

	colorTimeEncoder(time.Now(), &stringArrayEncoder{})
	colorCallerEncoder(zapcore.EntryCaller{}, &stringArrayEncoder{})
	colorNameEncoder("", &stringArrayEncoder{})
	colorNameEncoder("logger", &stringArrayEncoder{})
	for _, level := range []string{"debug", "info", "warn", "warning", "error", "nope"} {
		_ = parseLevel(level, zapcore.InfoLevel)
	}
	if ignoreSyncError(nil) != nil {
		t.Fatal("nil 同步错误应忽略")
	}
	if ignoreSyncError(&os.PathError{Op: "sync", Path: "stdout", Err: errors.New("bad")}) != nil {
		t.Fatal("stdout 同步错误应忽略")
	}
	if ignoreSyncError(errors.New("disk")) == nil {
		t.Fatal("普通同步错误应保留")
	}
	if _, err := (&colorConsoleWriter{w: failWriter{}}).Write([]byte("x")); err == nil {
		t.Fatal("控制台写入失败应返回错误")
	}
	if _, err := (&colorConsoleWriter{w: &bytes.Buffer{}}).Write([]byte("x")); err != nil {
		t.Fatal(err)
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

func TestAccessAndRotatorEdges(t *testing.T) {
	if captureRequestBody(nil, AccessConfig{RecordBody: true}, nil).OmittedReason != "" {
		t.Fatal("nil 请求应返回空采集结果")
	}
	imageReq := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("png"))
	imageReq.Header.Set("Content-Type", "image/png")
	if captureRequestBody(imageReq, AccessConfig{RecordBody: true}, nil).OmittedReason != omitImageContent {
		t.Fatal("图片正文应省略")
	}
	pdfReq := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("pdf"))
	pdfReq.Header.Set("Content-Type", "application/pdf")
	if captureRequestBody(pdfReq, AccessConfig{RecordBody: true}, nil).OmittedReason != omitBinaryContent {
		t.Fatal("二进制正文应省略")
	}
	emptyBody := httptest.NewRequest(http.MethodPost, "/", nil)
	emptyBody.Header.Set("Content-Type", "application/json")
	emptyBody.Body = nil
	if captureRequestBody(emptyBody, AccessConfig{RecordBody: true}, nil).OmittedReason != "" {
		t.Fatal("空 body 应直接返回")
	}
	failReq := httptest.NewRequest(http.MethodPost, "/", errReadCloser{})
	failReq.Header.Set("Content-Type", "text/plain")
	if captureRequestBody(failReq, AccessConfig{RecordBody: true, MaxBodyBytes: 0}, nil).OmittedReason != omitCaptureFailed {
		t.Fatal("读取失败应标记采集失败")
	}
	zeroLen := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("ab"))
	zeroLen.ContentLength = 0
	zeroLen.Header.Set("Content-Type", "text/plain")
	got := captureRequestBody(zeroLen, AccessConfig{RecordBody: true, MaxBodyBytes: 8}, nil)
	if got.Value != "ab" || got.Bytes != 2 {
		t.Fatalf("零 Content-Length 应回填字节数: %+v", got)
	}
	headers := http.Header{"X-Empty": {}, "X-Long": {string(bytes.Repeat([]byte("a"), 20))}}
	collected := collectHeaders(headers, AccessConfig{MaxFieldBytes: 4}, nil)
	if _, ok := collected["x-empty"]; ok || collected["x-long"] == "" {
		t.Fatalf("header 采集结果 = %v", collected)
	}

	recorder := httptest.NewRecorder()
	_, engine := gin.CreateTestContext(recorder)
	engine.GET("/body", func(c *gin.Context) {
		writer := newAccessBodyWriter(c.Writer, 4, true)
		if _, err := writer.WriteString("hello"); err != nil {
			t.Fatal(err)
		}
		writer.capture([]byte("more"))
		image := writer.captureResult(AccessConfig{RecordBody: true}, nil, "image/png")
		if image.OmittedReason != omitImageContent {
			t.Fatal("响应图片应省略")
		}
		disabled := writer.captureResult(AccessConfig{}, nil, "text/plain")
		if disabled.OmittedReason != omitAccessBodyDisabled {
			t.Fatal("关闭正文时应给出原因")
		}
	})
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/body", nil))

	if requestContext(nil) == nil || normalizePath("  ") != "/" || normalizePath("health") != "/health" {
		t.Fatal("路径和空上下文归一化不正确")
	}
	if !shouldKeepAccess(500, 0) || !shouldSample(0) {
		t.Fatal("错误状态和零采样率应保留日志")
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	disabledAccess := false
	router.Use(Middleware(AccessConfig{Enabled: &disabledAccess}))
	router.GET("/off", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/off", nil))

	sampled := gin.New()
	sampled.Use(Middleware(AccessConfig{
		RecordBody:   false,
		SkipPaths:    []string{"/skip"},
		MaxBodyBytes: 32,
	}))
	sampled.GET("/skip", func(c *gin.Context) { c.Status(http.StatusOK) })
	sampled.GET("/live", func(c *gin.Context) {
		c.Writer.Header().Set("Content-Type", "text/plain")
		c.String(http.StatusOK, "ok")
	})
	skipReq := httptest.NewRequest(http.MethodGet, "/skip", nil)
	sampled.ServeHTTP(httptest.NewRecorder(), skipReq)
	liveReq := httptest.NewRequest(http.MethodGet, "/live?q=1", nil)
	liveReq.Header.Set("User-Agent", "test-agent")
	sampled.ServeHTTP(httptest.NewRecorder(), liveReq)

	if _, err := NewFileRotator(RotatorOptions{}); err == nil {
		t.Fatal("空日志类型应失败")
	}
	if _, err := NewFileRotator(RotatorOptions{LogType: "info"}); err == nil {
		t.Fatal("非正滚动参数应失败")
	}
	if _, err := NewFileRotator(RotatorOptions{
		RootDir: t.TempDir(), LogType: "info", MaxSizeMB: 1, RetentionDays: 1, MaxBackups: 1, MaxTotalMB: 1, Timezone: "Not/AZone",
	}); err == nil {
		t.Fatal("非法时区应失败")
	}

	dir := t.TempDir()
	rot, err := NewFileRotator(RotatorOptions{
		RootDir: dir, LogType: "info", MaxSizeMB: 1, RetentionDays: 1, MaxBackups: 1, MaxTotalMB: 1, Compress: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	rot.mu.Lock()
	_ = rot.file.Close()
	rot.file = nil
	rot.mu.Unlock()
	if _, err := rot.Write([]byte("reopen")); err != nil {
		t.Fatal(err)
	}
	rot.mu.Lock()
	_ = rot.file.Close()
	rot.file = nil
	rot.mu.Unlock()
	if err := rot.Sync(); err != nil {
		t.Fatal(err)
	}
	rot.mu.Lock()
	active := rot.activePath()
	if rot.file != nil {
		_ = rot.file.Close()
		rot.file = nil
	}
	_ = os.Remove(active)
	rot.size = rot.maxSizeBytes
	rot.mu.Unlock()
	if _, err := rot.Write([]byte("rotated")); err != nil {
		t.Fatal(err)
	}
	var rotateErr error
	rot.reopenAfterRotateFailure(nil)
	rot.reopenAfterRotateFailure(&rotateErr)

	historyDir := filepath.Join(dir, "info")
	_ = os.MkdirAll(filepath.Join(historyDir, "nested"), 0o755)
	_ = os.WriteFile(filepath.Join(historyDir, "19990101-0000001.log"), []byte("old"), 0o644)
	_ = os.WriteFile(filepath.Join(historyDir, "20990101-0000001.log"), []byte("a"), 0o644)
	_ = os.WriteFile(filepath.Join(historyDir, "20990101-0000002.log"), []byte("b"), 0o644)
	_ = os.WriteFile(filepath.Join(historyDir, "20990101-note.log"), []byte("bad"), 0o644)
	if rot.scanNextSeq("20990101") < 3 {
		t.Fatal("历史序号应跳过已有文件")
	}
	missing := *rot
	missing.dir = filepath.Join(dir, "missing-seq")
	if missing.scanNextSeq("20990101") != 1 {
		t.Fatal("目录不可读时应从 1 开始")
	}
	rot.maxTotalBytes = 1
	if err := rot.cleanup(); err != nil {
		t.Fatal(err)
	}

	plain := filepath.Join(historyDir, "20990102-0000001.log")
	if err := os.WriteFile(plain, []byte("compress me"), 0o644); err != nil {
		t.Fatal(err)
	}
	compressHistoryFile(plain)
	compressHistoryFile(plain + ".gz")
	compressHistoryFile(filepath.Join(historyDir, "absent.log"))
	if err := rot.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rot.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := rot.Write([]byte("closed")); err == nil {
		t.Fatal("关闭后写入应失败")
	}
}
