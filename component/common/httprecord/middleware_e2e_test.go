// HTTP 访问记录中间件的端到端测试：真实 gin 引擎 + 记录收集服务
//
// @author zwei
package httprecord

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"gitee.com/hq_project/yomi-boot/component/common/token"
	"gitee.com/hq_project/yomi-boot/component/core/masking"
	"gitee.com/hq_project/yomi-boot/component/core/traceid"
)

// stubApp 测试用应用上下文
type stubApp struct{}

// Name 返回测试服务名
func (stubApp) Name() string { return "test-app" }

// Env 返回测试环境
func (stubApp) Env() string { return "test" }

// Version 返回测试版本
func (stubApp) Version() string { return "v1" }

// recorderService 收集访问记录的 HttpRecordService 桩
type recorderService struct {
	mu            sync.Mutex
	records       []*HttpRecord
	panicOnHandle bool
}

// Handle 收集记录，可按配置触发 panic
func (s *recorderService) Handle(_ context.Context, record *HttpRecord) {
	if s.panicOnHandle {
		panic("svc boom")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, record)
}

// snapshot 返回已收集记录的副本
func (s *recorderService) snapshot() []*HttpRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*HttpRecord(nil), s.records...)
}

// waitForRecords 轮询等待收集到 n 条记录
func waitForRecords(t *testing.T, s *recorderService, n int) []*HttpRecord {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if got := s.snapshot(); len(got) >= n {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("等待 %d 条记录超时，实际 %d 条", n, len(s.snapshot()))
	return nil
}

// newTestMaskService 构建覆盖脱敏场景的测试服务
func newTestMaskService(t *testing.T) *masking.Service {
	t.Helper()
	svc, err := masking.New(masking.Config{Rules: []masking.RuleConfig{
		{Keys: []string{"password", "token", "authorization"}, Strategy: masking.StrategyRedact},
	}})
	if err != nil {
		t.Fatalf("masking.New: %v", err)
	}
	return svc
}

// newTestEngine 构建带记录中间件的 gin 引擎与收集服务；mask 为注入的脱敏服务，nil 表示不脱敏
func newTestEngine(t *testing.T, cfg Config, svc HttpRecordService, mask *masking.Service) *gin.Engine {
	t.Helper()
	engine := gin.New()
	engine.Use(Middleware(svc, cfg, stubApp{}, mask))
	return engine
}

// authenticatedRequest 构造带会话与链路 ID 的请求
func authenticatedRequest(method, target, body string) *http.Request {
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	ctx := token.WithSession(req.Context(), &token.TokenSession{UserID: 7, ClientType: "admin"})
	ctx = traceid.ContextWithSpanContext(ctx, traceid.SpanContext{TraceID: "trace-1"})
	return req.WithContext(ctx)
}

// TestMiddlewareCapturesFullRecord 验证开启全部开关时记录的完整采集与脱敏
func TestMiddlewareCapturesFullRecord(t *testing.T) {
	rec := &recorderService{}
	cfg := Config{
		MiddlewareEnabled:  true,
		RecordRequestBody:  true,
		RecordResponseBody: true,
		MaskSensitive:      true,
		SkipPaths:          []string{"/health"},
	}
	engine := newTestEngine(t, cfg, rec, newTestMaskService(t))
	engine.POST("/api/v1/login/:id", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": "OK", "msg": "success", "token": "resp-secret"})
	})

	body := `{"name":"u","password":"p@ss","big":12345678901234567890}`
	req := authenticatedRequest(http.MethodPost, "/api/v1/login/42?access_token=secret&q=1", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer abc")
	req.Header.Set("User-Agent", "ua-test")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("请求应成功，实际 %d", w.Code)
	}

	records := waitForRecords(t, rec, 1)
	record := records[0]
	if record.ServiceType != "test-app" || record.ServiceVersion != "v1" {
		t.Fatalf("服务信息不符: %+v", record)
	}
	if record.TraceID != "trace-1" {
		t.Fatalf("链路 ID 不符: %q", record.TraceID)
	}
	if record.Method != http.MethodPost || record.Path != "/api/v1/login/42" || record.Route != "/api/v1/login/:id" {
		t.Fatalf("请求行不符: %+v", record)
	}
	if !strings.Contains(record.Query, "q=1") || strings.Contains(record.Query, "secret") {
		t.Fatalf("query 应脱敏且保留普通参数: %q", record.Query)
	}
	if record.RequestHeaders["Authorization"] != "***" || record.RequestHeaders["Content-Type"] != "application/json" {
		t.Fatalf("请求头脱敏不符: %+v", record.RequestHeaders)
	}
	if !strings.Contains(record.RequestBody, `"password":"***"`) || strings.Contains(record.RequestBody, "p@ss") {
		t.Fatalf("请求体脱敏不符: %q", record.RequestBody)
	}
	if !strings.Contains(record.RequestBody, "12345678901234567890") {
		t.Fatalf("大数应保留精度: %q", record.RequestBody)
	}
	if record.UserID != 7 || record.ClientType != "admin" {
		t.Fatalf("会话信息不符: %+v", record)
	}
	if record.StatusCode != http.StatusOK {
		t.Fatalf("状态码不符: %d", record.StatusCode)
	}
	if !strings.Contains(record.ResponseBody, `"token":"***"`) || strings.Contains(record.ResponseBody, "resp-secret") {
		t.Fatalf("响应体脱敏不符: %q", record.ResponseBody)
	}
	if record.BizCode != "OK" || record.BizMsg != "success" {
		t.Fatalf("业务码解析不符: %q %q", record.BizCode, record.BizMsg)
	}
	if record.RequestBodyTruncated || record.ResponseBodyTruncated {
		t.Fatal("未超限不应标记截断")
	}
}

// TestMiddlewareSkipsDisabledAndSkipPaths 验证总开关关闭与命中跳过路径时不采集
func TestMiddlewareSkipsDisabledAndSkipPaths(t *testing.T) {
	rec := &recorderService{}
	cfg := Config{
		MiddlewareEnabled: true,
		SkipPaths:         []string{"/health"},
	}
	engine := newTestEngine(t, cfg, rec, nil)
	engine.GET("/health", func(c *gin.Context) { c.Status(http.StatusOK) })
	engine.GET("/api/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	// 总开关关闭
	disabledRec := &recorderService{}
	disabledCfg := cfg
	disabledCfg.MiddlewareEnabled = false
	disabledEngine := newTestEngine(t, disabledCfg, disabledRec, nil)
	disabledEngine.GET("/api/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health/", nil))
	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))
	disabledEngine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/ping", nil))
	time.Sleep(100 * time.Millisecond)
	if got := rec.snapshot(); len(got) != 0 {
		t.Fatalf("跳过路径不应采集，实际 %d 条", len(got))
	}
	if got := disabledRec.snapshot(); len(got) != 0 {
		t.Fatalf("总开关关闭不应采集，实际 %d 条", len(got))
	}

	// 未命中跳过路径的正常采集
	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/ping", nil))
	waitForRecords(t, rec, 1)
}

// TestMiddlewareWithoutMasking 验证 mask_sensitive 开关关闭时不调用脱敏组件
// （即使注入了脱敏服务也原文直出：开关判定内聚在组件）
func TestMiddlewareWithoutMasking(t *testing.T) {
	rec := &recorderService{}
	cfg := Config{
		MiddlewareEnabled:  true,
		RecordRequestBody:  true,
		RecordResponseBody: true,
	}
	engine := newTestEngine(t, cfg, rec, newTestMaskService(t))
	engine.POST("/plain", func(c *gin.Context) {
		c.String(http.StatusOK, "plain-body")
	})

	req := authenticatedRequest(http.MethodPost, "/plain?token=raw-secret", `{"password":"raw-pass"}`)
	req.Header.Set("Authorization", "Bearer raw")
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(httptest.NewRecorder(), req)

	records := waitForRecords(t, rec, 1)
	record := records[0]
	if record.Query != "token=raw-secret" {
		t.Fatalf("query 应原样保留: %q", record.Query)
	}
	if !strings.Contains(record.RequestBody, "raw-pass") {
		t.Fatalf("请求体应原样保留: %q", record.RequestBody)
	}
	if record.RequestHeaders["Authorization"] != "Bearer raw" {
		t.Fatalf("敏感头应原样保留: %v", record.RequestHeaders["Authorization"])
	}
	if record.ResponseBody != "plain-body" {
		t.Fatalf("WriteString 路径响应体应采集: %q", record.ResponseBody)
	}
	if record.BizCode != "" {
		t.Fatalf("非 JSON 业务响应不应解析出业务码: %q", record.BizCode)
	}
}

// TestMiddlewareTruncationFlags 验证请求与响应体超限截断：仅保留标记，正文不落记录
// （截断残片无法按字段脱敏，为避免敏感字段原文落盘整段丢弃）
func TestMiddlewareTruncationFlags(t *testing.T) {
	rec := &recorderService{}
	four := 4
	cfg := Config{
		MiddlewareEnabled:   true,
		RecordRequestBody:   true,
		RecordResponseBody:  true,
		MaxRequestBodySize:  &four,
		MaxResponseBodySize: &four,
	}
	engine := newTestEngine(t, cfg, rec, nil)
	engine.POST("/big", func(c *gin.Context) {
		_, _ = c.Writer.Write([]byte("1234567890"))
	})

	engine.ServeHTTP(httptest.NewRecorder(), authenticatedRequest(http.MethodPost, "/big", "abcdefghij"))

	records := waitForRecords(t, rec, 1)
	record := records[0]
	if !record.RequestBodyTruncated || record.RequestBody != "" {
		t.Fatalf("截断请求体应仅留标记不落正文: %q %v", record.RequestBody, record.RequestBodyTruncated)
	}
	if !record.ResponseBodyTruncated || record.ResponseBody != "" {
		t.Fatalf("截断响应体应仅留标记不落正文: %q %v", record.ResponseBody, record.ResponseBodyTruncated)
	}
}

// TestMiddlewareSafeInvokeRecoversPanic 验证记录处理 panic 不影响 HTTP 主链路
func TestMiddlewareSafeInvokeRecoversPanic(t *testing.T) {
	rec := &recorderService{panicOnHandle: true}
	cfg := Config{MiddlewareEnabled: true}
	engine := newTestEngine(t, cfg, rec, nil)
	engine.GET("/boom", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/boom", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("记录处理 panic 不应影响响应，实际 %d", w.Code)
	}
}

// TestMiddlewareNilService 验证未注入服务时请求正常完成
func TestMiddlewareNilService(t *testing.T) {
	cfg := Config{MiddlewareEnabled: true}
	engine := newTestEngine(t, cfg, nil, nil)
	engine.GET("/no-svc", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/no-svc", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("无服务时请求应正常完成，实际 %d", w.Code)
	}
}

// TestMiddlewareNilAppPanics 验证缺失应用上下文时 Fail Fast
func TestMiddlewareNilAppPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("app 为 nil 应 panic")
		}
	}()
	Middleware(nil, Config{}, nil, nil)
}
