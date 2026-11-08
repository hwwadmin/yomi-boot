// Package logger 配置与滚动单元测试
//
// @author zwei
package logger

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/hwwadmin/yomi-boot/component/core/masking"
	"github.com/hwwadmin/yomi-boot/component/core/traceid"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

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

// testRotation 返回测试用滚动配置
func testRotation() RotationConfig {
	return RotationConfig{
		MaxSizeMB:     50,
		RetentionDays: 3,
		MaxBackups:    30,
		MaxTotalMB:    2048,
	}
}

// TestConfig_Normalize_RequiresRotation 验证 Normalize 要求 Rotation 配置
func TestConfig_Normalize_RequiresRotation(t *testing.T) {
	cfg := Config{}
	if err := cfg.Normalize(); err == nil {
		t.Fatal("expected rotation required error")
	}
	cfg.Rotation = testRotation()
	if err := cfg.Normalize(); err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if cfg.Dir != "logs" || cfg.Level != "info" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if !cfg.ConsoleEnabledOrDefault() || !cfg.AsyncEnabledOrDefault() {
		t.Fatal("console/async should default enabled")
	}
	if cfg.AsyncQueueSize != 4096 {
		t.Fatalf("async_queue_size default want 4096, got %d", cfg.AsyncQueueSize)
	}
	if !cfg.Access.EnabledOrDefault() {
		t.Fatal("access should default enabled")
	}
	if cfg.Access.SampleRate != 1 {
		t.Fatalf("sample_rate default want 1, got %v", cfg.Access.SampleRate)
	}
}

// TestAccessConfig_SampleRateOrDefault 验证 SampleRateOrDefault 默认值与边界
func TestAccessConfig_SampleRateOrDefault(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{0, 1},
		{-1, 1},
		{1.5, 1},
		{1, 1},
		{0.1, 0.1},
	}
	for _, tc := range cases {
		got := (AccessConfig{SampleRate: tc.in}).SampleRateOrDefault()
		if got != tc.want {
			t.Fatalf("in=%v want %v got %v", tc.in, tc.want, got)
		}
	}
}

// TestConfig_Normalize_ReservedFields 验证 Normalize 拒绝保留字段
func TestConfig_Normalize_ReservedFields(t *testing.T) {
	cfg := Config{
		Rotation: testRotation(),
		Fields:   map[string]string{traceid.Traceparent: "x"},
	}
	if err := cfg.Normalize(); err == nil {
		t.Fatal("expected reserved field error")
	}
}

// TestInitLogger_AccessCreateFailureStopsSink 验证 access 目录创建失败时返回错误并停掉已启动的控制台协程
func TestInitLogger_AccessCreateFailureStopsSink(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	before := runtime.NumGoroutine()
	hubBefore := defaultHub.Load()
	closeFn, err := InitLogger("test-svc", "test", Config{
		Dir:      blocker,
		Level:    "info",
		Rotation: testRotation(),
	}, nil)
	if err == nil {
		if closeFn != nil {
			_ = closeFn()
		}
		t.Fatal("期望 access 目录创建失败")
	}
	if !strings.Contains(err.Error(), "初始化 access 日志失败") {
		t.Fatalf("错误应说明 access 初始化失败，实际 %v", err)
	}
	if closeFn != nil {
		t.Fatal("失败时不应返回清理函数")
	}
	if defaultHub.Load() != hubBefore {
		t.Fatal("失败的初始化替换了当前日志枢纽")
	}

	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() != before {
		if time.Now().After(deadline) {
			t.Fatalf("控制台协程未停：之前 %d，现在 %d", before, runtime.NumGoroutine())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestFileSplit_ELKFieldsAndStaticFields 验证 info/access/error 分文件及 ELK 字段
func TestFileSplit_ELKFieldsAndStaticFields(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Dir:      dir,
		Level:    "info",
		Rotation: testRotation(),
		Fields: map[string]string{
			"country_code": "pk",
			"instance_id":  "i-test",
			"empty_skip":   "",
		},
	}
	closeFn, err := InitLogger("test-svc", "test", cfg, nil)
	if err != nil {
		t.Fatalf("InitLogger: %v", err)
	}
	defer func() { _ = closeFn() }()

	ctx := context.Background()
	Info(ctx, "business event", "module", "demo")
	Access(ctx, "http request completed", "method", "GET", "path", "/api/ping", "status", 200)
	Error(ctx, "boom", "error_type", "test")
	_ = Sync()

	infoPath := filepath.Join(dir, "info", "info.log")
	accessPath := filepath.Join(dir, "access", "access.log")
	errorPath := filepath.Join(dir, "error", "error.log")

	assertFileContains(t, infoPath, "business event")
	assertFileNotContains(t, infoPath, "http request completed")
	assertFileContains(t, accessPath, "http request completed")
	assertFileContains(t, errorPath, "boom")

	assertJSONField(t, accessPath, "log_type", "access")
	assertJSONField(t, infoPath, "log_type", "info")
	assertJSONField(t, errorPath, "log_type", "error")

	assertJSONField(t, infoPath, "message", "business event")
	assertJSONField(t, infoPath, "level", "info")
	assertFileNotContains(t, infoPath, `"traceparent"`)
	assertFileNotContains(t, infoPath, `"trace_id"`)
	assertFileNotContains(t, infoPath, `"span_id"`)
	assertJSONField(t, infoPath, "country_code", "pk")
	assertJSONField(t, infoPath, "instance_id", "i-test")
	assertFileNotContains(t, infoPath, `"msg"`)
	assertFileNotContains(t, infoPath, "empty_skip")
}

// TestTraceparentField_ContextWinsOrCallerKept 验证没有链路时保留调用方的 traceparent，有链路时用上下文的那串
func TestTraceparentField_ContextWinsOrCallerKept(t *testing.T) {
	dir := t.TempDir()
	closeFn, err := InitLogger("test-svc", "test", Config{
		Dir:      dir,
		Level:    "info",
		Rotation: testRotation(),
	}, nil)
	if err != nil {
		t.Fatalf("InitLogger: %v", err)
	}
	defer func() { _ = closeFn() }()

	callerTP := "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01"
	contextTP := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	Info(context.Background(), "from caller", traceid.Traceparent, callerTP)
	ctx := traceid.ContextWithTraceparent(context.Background(), contextTP)
	Info(ctx, "from context", traceid.Traceparent, callerTP)
	Info(context.Background(), "blank skipped", traceid.Traceparent, "  ")
	_ = Sync()

	infoPath := filepath.Join(dir, "info", "info.log")
	lines := readLines(t, infoPath)
	if len(lines) < 3 {
		t.Fatalf("日志行数 = %d，内容 %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], `"traceparent":"`+callerTP+`"`) || !strings.Contains(lines[0], "from caller") {
		t.Fatalf("没有链路时应留下调用方的 traceparent: %s", lines[0])
	}
	if !strings.Contains(lines[1], `"traceparent":"`+contextTP+`"`) || strings.Contains(lines[1], callerTP) {
		t.Fatalf("有链路时应只用上下文的 traceparent: %s", lines[1])
	}
	if strings.Contains(lines[2], `"traceparent"`) {
		t.Fatalf("空白 traceparent 不应写入: %s", lines[2])
	}
}

// readLines 读出日志文件的非空行
func readLines(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var lines []string
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// TestMiddleware_InboundOutboundPair 验证 access 中间件成对写入入站/出站日志
func TestMiddleware_InboundOutboundPair(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	cfg := Config{
		Dir:           dir,
		Level:         "info",
		Rotation:      testRotation(),
		MaskSensitive: true,
		Access: AccessConfig{
			SkipPaths:  []string{},
			RecordBody: true,
		},
	}
	maskSvc, err := masking.New(masking.Config{Rules: []masking.RuleConfig{
		{Keys: []string{"authorization", "password"}, Strategy: masking.StrategyRedact},
	}})
	if err != nil {
		t.Fatalf("masking.New: %v", err)
	}
	closeFn, err := InitLogger("test-svc", "test", cfg, maskSvc)
	if err != nil {
		t.Fatalf("InitLogger: %v", err)
	}
	defer func() { _ = closeFn() }()

	r := gin.New()
	r.Use(func(c *gin.Context) {
		// 模拟 traceid.Middleware：入口 root span，inbound/outbound 共用
		c.Request = c.Request.WithContext(traceid.ContextWithSpanContext(c.Request.Context(), traceid.SpanContext{
			TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
			SpanID:  "00f067aa0ba902b7",
		}))
		c.Next()
	})
	r.Use(Middleware(cfg.Access))
	r.POST("/api/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true, "password": "secret"})
	})

	w := httptest.NewRecorder()
	body := `{"name":"a","password":"p@ss"}`
	req := httptest.NewRequest(http.MethodPost, "/api/ping", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret-token")
	r.ServeHTTP(w, req)
	_ = Sync()

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}

	accessPath := filepath.Join(dir, "access", "access.log")
	b, err := os.ReadFile(accessPath)
	if err != nil {
		t.Fatalf("read access: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 access lines (inbound+outbound), got %d content=%s", len(lines), string(b))
	}
	if !strings.Contains(lines[0], `"direction":"inbound"`) || !strings.Contains(lines[0], "http inbound") {
		t.Fatalf("line0 want inbound, got %s", lines[0])
	}
	if !strings.Contains(lines[0], `"request_headers"`) {
		t.Fatalf("inbound missing request_headers: %s", lines[0])
	}
	if !strings.Contains(lines[0], `"***"`) {
		t.Fatalf("inbound should redact authorization: %s", lines[0])
	}
	if !strings.Contains(lines[0], `"request_body"`) || !strings.Contains(lines[0], `"***"`) {
		t.Fatalf("inbound should have redacted request_body: %s", lines[0])
	}
	if !strings.Contains(lines[1], `"direction":"outbound"`) || !strings.Contains(lines[1], "http outbound") {
		t.Fatalf("line1 want outbound, got %s", lines[1])
	}
	if !strings.Contains(lines[1], `"status":200`) || !strings.Contains(lines[1], "duration_ms") {
		t.Fatalf("outbound should have status/duration, got %s", lines[1])
	}
	if !strings.Contains(lines[1], `"response_body"`) {
		t.Fatalf("outbound missing response_body: %s", lines[1])
	}
	if strings.Contains(lines[1], `"password":"secret"`) {
		t.Fatalf("response password should be redacted: %s", lines[1])
	}
	for i, line := range lines {
		if !strings.Contains(line, `"traceparent":"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"`) {
			t.Fatalf("line%d missing traceparent: %s", i, line)
		}
		if strings.Contains(line, `"request_id"`) || strings.Contains(line, `"trace_id"`) || strings.Contains(line, `"span_id"`) {
			t.Fatalf("line%d 不应再拆追踪字段: %s", i, line)
		}
	}
	assertFileNotContains(t, filepath.Join(dir, "info", "info.log"), "http inbound")
	assertFileNotContains(t, filepath.Join(dir, "info", "info.log"), "http outbound")
}

// TestMiddleware_ErrorAlwaysLogged 验证错误响应不受采样率影响
func TestMiddleware_ErrorAlwaysLogged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	cfg := Config{
		Dir:      dir,
		Level:    "info",
		Rotation: testRotation(),
		Access: AccessConfig{
			SkipPaths:  []string{},
			SampleRate: 0.0001,
		},
	}
	closeFn, err := InitLogger("test-svc", "test", cfg, nil)
	if err != nil {
		t.Fatalf("InitLogger: %v", err)
	}
	defer func() { _ = closeFn() }()

	prev := accessRandFloat64
	accessRandFloat64 = func() float64 { return 0.999 } // 永不命中成功采样
	defer func() { accessRandFloat64 = prev }()

	r := gin.New()
	r.Use(Middleware(cfg.Access))
	r.GET("/fail", func(c *gin.Context) {
		c.JSON(http.StatusInternalServerError, gin.H{"err": "boom"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	r.ServeHTTP(w, req)
	_ = Sync()

	accessPath := filepath.Join(dir, "access", "access.log")
	b, err := os.ReadFile(accessPath)
	if err != nil {
		t.Fatalf("read access: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 {
		t.Fatalf("error must keep inbound+outbound, got %d content=%s", len(lines), string(b))
	}
	if !strings.Contains(lines[1], `"status":500`) {
		t.Fatalf("want status 500, got %s", lines[1])
	}
}

// TestMiddleware_SampleDropsOK 验证成功请求可被采样丢弃
func TestMiddleware_SampleDropsOK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	cfg := Config{
		Dir:      dir,
		Level:    "info",
		Rotation: testRotation(),
		Access: AccessConfig{
			SkipPaths:  []string{},
			SampleRate: 0.1,
		},
	}
	closeFn, err := InitLogger("test-svc", "test", cfg, nil)
	if err != nil {
		t.Fatalf("InitLogger: %v", err)
	}
	defer func() { _ = closeFn() }()

	prev := accessRandFloat64
	accessRandFloat64 = func() float64 { return 0.99 } // 不命中 0.1
	defer func() { accessRandFloat64 = prev }()

	r := gin.New()
	r.Use(Middleware(cfg.Access))
	r.GET("/ok", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	r.ServeHTTP(w, req)
	_ = Sync()

	accessPath := filepath.Join(dir, "access", "access.log")
	b, err := os.ReadFile(accessPath)
	if err == nil && strings.TrimSpace(string(b)) != "" {
		t.Fatalf("sampled-out 200 should not write access, got %s", string(b))
	}
}

// TestMiddleware_OutboundPanicLogged 验证出站落盘段 panic 被兜底记录且不影响响应
func TestMiddleware_OutboundPanicLogged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	cfg := Config{
		Dir:      dir,
		Level:    "info",
		Rotation: testRotation(),
		Access: AccessConfig{
			SkipPaths:  []string{},
			SampleRate: 0.5,
		},
	}
	closeFn, err := InitLogger("test-svc", "test", cfg, nil)
	if err != nil {
		t.Fatalf("InitLogger: %v", err)
	}
	defer func() { _ = closeFn() }()

	// 让采样判定在出站段 panic，模拟落盘代码自身的 bug
	prev := accessRandFloat64
	accessRandFloat64 = func() float64 { panic("sample boom") }
	defer func() { accessRandFloat64 = prev }()

	r := gin.New()
	r.Use(Middleware(cfg.Access))
	r.GET("/ok", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	r.ServeHTTP(w, req)
	_ = Sync()

	if w.Code != http.StatusOK {
		t.Fatalf("出站段 panic 不应影响响应: status=%d", w.Code)
	}
	errorPath := filepath.Join(dir, "error", "error.log")
	assertFileContains(t, errorPath, "access 出站落盘 panic")
	assertFileContains(t, errorPath, "sample boom")
}

// TestMiddleware_BodyOmittedBinary 验证二进制 body 被省略
func TestMiddleware_BodyOmittedBinary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	cfg := Config{
		Dir:      dir,
		Level:    "info",
		Rotation: testRotation(),
		Access: AccessConfig{
			SkipPaths:  []string{},
			RecordBody: true,
		},
	}
	closeFn, err := InitLogger("test-svc", "test", cfg, nil)
	if err != nil {
		t.Fatalf("InitLogger: %v", err)
	}
	defer func() { _ = closeFn() }()

	r := gin.New()
	r.Use(Middleware(cfg.Access))
	r.POST("/api/upload", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/upload", strings.NewReader("fake-bytes"))
	req.Header.Set("Content-Type", "application/octet-stream")
	r.ServeHTTP(w, req)
	_ = Sync()

	accessPath := filepath.Join(dir, "access", "access.log")
	b, err := os.ReadFile(accessPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	line := strings.Split(strings.TrimSpace(string(b)), "\n")[0]
	if !strings.Contains(line, `"body_omitted_reason":"binary_content"`) {
		t.Fatalf("want binary omit reason, got %s", line)
	}
	if strings.Contains(line, "fake-bytes") {
		t.Fatalf("binary body should not be logged: %s", line)
	}
}

// TestMiddleware_BodyTooLargeNotLogged 验证超限截断的 body 不落正文：
// 截断残片无法按字段脱敏，access 日志只保留 body_omitted_reason 与字节数
func TestMiddleware_BodyTooLargeNotLogged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	cfg := Config{
		Dir:           dir,
		Level:         "info",
		MaskSensitive: true,
		Rotation:      testRotation(),
		Access: AccessConfig{
			SkipPaths:    []string{},
			RecordBody:   true,
			MaxBodyBytes: 16,
		},
	}
	maskSvc, err := masking.New(masking.Config{Rules: []masking.RuleConfig{
		{Keys: []string{"password"}, Strategy: masking.StrategyRedact},
	}})
	if err != nil {
		t.Fatalf("masking.New: %v", err)
	}
	closeFn, err := InitLogger("test-svc", "test", cfg, maskSvc)
	if err != nil {
		t.Fatalf("InitLogger: %v", err)
	}
	defer func() { _ = closeFn() }()

	r := gin.New()
	r.Use(Middleware(cfg.Access))
	r.POST("/api/big-req", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	r.POST("/api/big-resp", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"password": "resp-secret", "pad": strings.Repeat("x", 64)})
	})

	bigReq := `{"password":"super-secret-value","pad":"` + strings.Repeat("x", 64) + `"}`
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/big-req", strings.NewReader(bigReq)))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/big-resp", strings.NewReader(`{"password":"x"}`)))
	_ = Sync()

	accessPath := filepath.Join(dir, "access", "access.log")
	b, err := os.ReadFile(accessPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	content := string(b)
	if strings.Contains(content, "super-secret-value") || strings.Contains(content, "resp-secret") {
		t.Fatalf("截断正文不应落盘: %s", content)
	}
	// 请求体截断（第一条 inbound）与响应体截断（第二条 outbound）各产生一条省略原因
	if got := strings.Count(content, `"body_omitted_reason":"body_too_large"`); got != 2 {
		t.Fatalf("应恰有两条 body_too_large，实际 %d: %s", got, content)
	}
	// 未超限的请求体仍正常脱敏落盘（JSON 行内引号转义为 \"）
	if !strings.Contains(content, `\"password\":\"***\"`) {
		t.Fatalf("未超限请求体应正常脱敏: %s", content)
	}
}

// TestCollectHeaders_UTF8Truncate 验证超长 header 值按 UTF-8 边界截断，不切碎多字节字符
func TestCollectHeaders_UTF8Truncate(t *testing.T) {
	cfg := AccessConfig{MaxFieldBytes: 10}
	h := http.Header{}
	h.Set("X-Long-CN", strings.Repeat("中", 10)) // 30 字节，10 字节处落在第 4 个"中"的中间
	h.Set("X-Long-ASCII", strings.Repeat("a", 30))

	// mask 传 nil：mask_sensitive=false 时生产即此形态，顺带覆盖 nil 安全
	out := collectHeaders(h, cfg, nil)

	got := out["x-long-cn"]
	body := strings.TrimSuffix(got, truncatedPlaceholder)
	if !strings.HasSuffix(got, truncatedPlaceholder) || !utf8.ValidString(body) {
		t.Fatalf("中文截断应回退到 UTF-8 边界并以占位符结尾: %q", got)
	}
	if body != strings.Repeat("中", 3) {
		t.Fatalf("应保留 3 个完整字符（9 字节），实际 %d 字节: %q", len(body), body)
	}

	gotASCII := out["x-long-ascii"]
	if gotASCII != strings.Repeat("a", 10)+truncatedPlaceholder {
		t.Fatalf("ASCII 截断应精确到上限: %q", gotASCII)
	}
}

// TestFileRotator_SizeRotate 验证按大小滚动生成历史文件
func TestFileRotator_SizeRotate(t *testing.T) {
	dir := t.TempDir()
	rot, err := NewFileRotator(RotatorOptions{
		RootDir:       dir,
		LogType:       "info",
		MaxSizeMB:     1,
		RetentionDays: 3,
		MaxBackups:    10,
		MaxTotalMB:    100,
		Compress:      false,
		Timezone:      "Asia/Singapore",
	})
	if err != nil {
		t.Fatalf("NewFileRotator: %v", err)
	}
	defer func() { _ = rot.Close() }()

	rot.mu.Lock()
	rot.maxSizeBytes = 64
	rot.mu.Unlock()

	payload := strings.Repeat("x", 40) + "\n"
	if _, err := rot.Write([]byte(payload)); err != nil {
		t.Fatalf("write1: %v", err)
	}
	if _, err := rot.Write([]byte(payload)); err != nil {
		t.Fatalf("write2: %v", err)
	}
	if _, err := rot.Write([]byte(payload)); err != nil {
		t.Fatalf("write3: %v", err)
	}
	_ = rot.Sync()

	entries, err := os.ReadDir(filepath.Join(dir, "info"))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	var hasHistory bool
	today := time.Now().In(rot.location).Format("20060102")
	for _, e := range entries {
		name := e.Name()
		if name == "info.log" {
			continue
		}
		if strings.HasPrefix(name, today+"-") && strings.HasSuffix(name, ".log") {
			hasHistory = true
		}
	}
	if !hasHistory {
		t.Fatalf("expected history file, entries=%v", names(entries))
	}
}

// TestFileRotator_RenameFailureKeepsWriting 验证改名失败只丢掉这一笔，下一笔仍能滚动。
func TestFileRotator_RenameFailureKeepsWriting(t *testing.T) {
	dir := t.TempDir()
	rot, err := NewFileRotator(RotatorOptions{
		RootDir:       dir,
		LogType:       "info",
		MaxSizeMB:     1,
		RetentionDays: 3,
		MaxBackups:    10,
		MaxTotalMB:    100,
		Compress:      false,
		Timezone:      "Asia/Singapore",
	})
	if err != nil {
		t.Fatalf("NewFileRotator: %v", err)
	}
	defer func() { _ = rot.Close() }()

	rot.mu.Lock()
	rot.maxSizeBytes = 64
	rot.mu.Unlock()

	payload := []byte(strings.Repeat("x", 40) + "\n")
	if _, err := rot.Write(payload); err != nil {
		t.Fatalf("write1: %v", err)
	}

	today := time.Now().In(rot.location).Format("20060102")
	logDir := filepath.Join(dir, "info")
	// 占住第一个历史名，让这次 rename 失败
	if err := os.Mkdir(filepath.Join(logDir, today+"-0000001.log"), 0o755); err != nil {
		t.Fatalf("mkdir conflict: %v", err)
	}

	if _, err := rot.Write(payload); err == nil || !strings.Contains(err.Error(), "滚动日志 rename 失败") {
		t.Fatalf("want rename error, got %v", err)
	}

	recovered := []byte(strings.Repeat("y", 40) + "\n")
	if _, err := rot.Write(recovered); err != nil {
		t.Fatalf("write after rename failure: %v", err)
	}
	if err := rot.Sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}

	history := filepath.Join(logDir, today+"-0000002.log")
	gotHistory, err := os.ReadFile(history)
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	if string(gotHistory) != string(payload) {
		t.Fatalf("history = %q, want first payload", gotHistory)
	}
	gotActive, err := os.ReadFile(filepath.Join(logDir, "info.log"))
	if err != nil {
		t.Fatalf("read active: %v", err)
	}
	if string(gotActive) != string(recovered) {
		t.Fatalf("active = %q, want recovered payload", gotActive)
	}

	if err := rot.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := rot.Write(payload); err == nil || !strings.Contains(err.Error(), "日志文件已关闭") {
		t.Fatalf("closed write = %v, want 日志文件已关闭", err)
	}
}

// TestFileRotator_DefaultUsesProcessTimezone 验证未指定时区时跟随进程时区
func TestFileRotator_DefaultUsesProcessTimezone(t *testing.T) {
	original := time.Local
	t.Cleanup(func() { time.Local = original })
	loc := time.FixedZone("test-zone", 5*3600)
	time.Local = loc

	rot, err := NewFileRotator(RotatorOptions{
		RootDir:       t.TempDir(),
		LogType:       "info",
		MaxSizeMB:     1,
		RetentionDays: 1,
		MaxBackups:    1,
		MaxTotalMB:    1,
	})
	if err != nil {
		t.Fatalf("NewFileRotator: %v", err)
	}
	defer func() { _ = rot.Close() }()

	if rot.location != loc {
		t.Fatalf("location = %v, want process time.Local", rot.location)
	}
}

// names 提取目录项名称列表
func names(entries []os.DirEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// assertFileContains 断言文件包含指定内容
func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(b), want) {
		t.Fatalf("%s missing %q, content=%s", path, want, string(b))
	}
}

// assertFileNotContains 断言文件不包含指定内容
func assertFileNotContains(t *testing.T, path, ban string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if strings.Contains(string(b), ban) {
		t.Fatalf("%s should not contain %q, content=%s", path, ban, string(b))
	}
}

// assertJSONField 断言日志首行 JSON 指定字段值
func assertJSONField(t *testing.T, path, key, want string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		t.Fatalf("empty file %s", path)
	}
	var m map[string]any
	if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
		t.Fatalf("json %s: %v body=%s", path, err, sc.Text())
	}
	got, _ := m[key].(string)
	if got != want {
		t.Fatalf("%s field %s=%q want %q full=%v", path, key, got, want, m)
	}
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
