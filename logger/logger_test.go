// Package logger 配置与滚动单元测试
//
// @author zwei
package logger

import (
	"bufio"
	"context"
	"encoding/json"
	"gitee.com/hq_project/yomi-boot/traceid"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

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
	if cfg.Dir != "logs" || cfg.Level != "info" || cfg.ConsoleLevel != "debug" {
		t.Fatalf("unexpected defaults: %+v", cfg)
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
		Fields:   map[string]string{"trace_id": "x"},
	}
	if err := cfg.Normalize(); err == nil {
		t.Fatal("expected reserved field error")
	}
}

// TestFileSplit_ELKFieldsAndStaticFields 验证 info/access/error 分文件及 ELK 字段
func TestFileSplit_ELKFieldsAndStaticFields(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Dir:          dir,
		Level:        "info",
		ConsoleLevel: "error",
		Timezone:     "Asia/Singapore",
		Rotation:     testRotation(),
		Fields: map[string]string{
			"country_code": "pk",
			"instance_id":  "i-test",
			"empty_skip":   "",
		},
	}
	closeFn, err := InitLogger("test-svc", "test", cfg)
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
	assertJSONField(t, infoPath, "trace_id", "-1")
	assertJSONField(t, infoPath, "span_id", "-1")
	assertJSONField(t, infoPath, "country_code", "pk")
	assertJSONField(t, infoPath, "instance_id", "i-test")
	assertFileNotContains(t, infoPath, `"msg"`)
	assertFileNotContains(t, infoPath, `"traceID"`)
	assertFileNotContains(t, infoPath, "empty_skip")
}

// TestAccessMiddleware_InboundOutboundPair 验证 access 中间件成对写入入站/出站日志
func TestAccessMiddleware_InboundOutboundPair(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	cfg := Config{
		Dir:           dir,
		Level:         "info",
		ConsoleLevel:  "error",
		Timezone:      "Asia/Singapore",
		Rotation:      testRotation(),
		SensitiveKeys: []string{"authorization", "password"},
		Access: AccessConfig{
			SkipPaths:  []string{},
			RecordBody: true,
		},
	}
	closeFn, err := InitLogger("test-svc", "test", cfg)
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
	r.Use(AccessMiddleware(cfg.Access))
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
	if !strings.Contains(lines[0], "[REDACTED]") {
		t.Fatalf("inbound should redact authorization: %s", lines[0])
	}
	if !strings.Contains(lines[0], `"request_body"`) || !strings.Contains(lines[0], "[REDACTED]") {
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
		if !strings.Contains(line, `"trace_id":"4bf92f3577b34da6a3ce929d0e0e4736"`) {
			t.Fatalf("line%d missing trace_id: %s", i, line)
		}
		if !strings.Contains(line, `"span_id":"00f067aa0ba902b7"`) {
			t.Fatalf("line%d missing shared root span_id: %s", i, line)
		}
	}
	assertFileNotContains(t, filepath.Join(dir, "info", "info.log"), "http inbound")
	assertFileNotContains(t, filepath.Join(dir, "info", "info.log"), "http outbound")
}

// TestAccessMiddleware_ErrorAlwaysLogged 验证错误响应不受采样率影响
func TestAccessMiddleware_ErrorAlwaysLogged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	cfg := Config{
		Dir:          dir,
		Level:        "info",
		ConsoleLevel: "error",
		Timezone:     "Asia/Singapore",
		Rotation:     testRotation(),
		Access: AccessConfig{
			SkipPaths:  []string{},
			SampleRate: 0.0001,
		},
	}
	closeFn, err := InitLogger("test-svc", "test", cfg)
	if err != nil {
		t.Fatalf("InitLogger: %v", err)
	}
	defer func() { _ = closeFn() }()

	prev := accessRandFloat64
	accessRandFloat64 = func() float64 { return 0.999 } // 永不命中成功采样
	defer func() { accessRandFloat64 = prev }()

	r := gin.New()
	r.Use(AccessMiddleware(cfg.Access))
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

// TestAccessMiddleware_SampleDropsOK 验证成功请求可被采样丢弃
func TestAccessMiddleware_SampleDropsOK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	cfg := Config{
		Dir:          dir,
		Level:        "info",
		ConsoleLevel: "error",
		Timezone:     "Asia/Singapore",
		Rotation:     testRotation(),
		Access: AccessConfig{
			SkipPaths:  []string{},
			SampleRate: 0.1,
		},
	}
	closeFn, err := InitLogger("test-svc", "test", cfg)
	if err != nil {
		t.Fatalf("InitLogger: %v", err)
	}
	defer func() { _ = closeFn() }()

	prev := accessRandFloat64
	accessRandFloat64 = func() float64 { return 0.99 } // 不命中 0.1
	defer func() { accessRandFloat64 = prev }()

	r := gin.New()
	r.Use(AccessMiddleware(cfg.Access))
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

// TestAccessMiddleware_BodyOmittedBinary 验证二进制 body 被省略
func TestAccessMiddleware_BodyOmittedBinary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	cfg := Config{
		Dir:          dir,
		Level:        "info",
		ConsoleLevel: "error",
		Timezone:     "Asia/Singapore",
		Rotation:     testRotation(),
		Access: AccessConfig{
			SkipPaths:  []string{},
			RecordBody: true,
		},
	}
	closeFn, err := InitLogger("test-svc", "test", cfg)
	if err != nil {
		t.Fatalf("InitLogger: %v", err)
	}
	defer func() { _ = closeFn() }()

	r := gin.New()
	r.Use(AccessMiddleware(cfg.Access))
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
