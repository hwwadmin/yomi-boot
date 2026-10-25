// HTTP 访问记录脱敏链路的单元测试：经注入的统一脱敏服务按 masking 规则处理
//
// @author zwei
package httprecord

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/core/masking"

	"github.com/gin-gonic/gin"
)

// TestCollectRequestHeadersWithMasking 验证请求头收集按注入服务脱敏
func TestCollectRequestHeadersWithMasking(t *testing.T) {
	mask, err := masking.New(masking.Config{Rules: []masking.RuleConfig{
		{Keys: []string{"authorization"}, Strategy: masking.StrategyRedact},
	}})
	if err != nil {
		t.Fatalf("masking.New: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.Header.Set("Authorization", "Bearer abc")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Add("X-Multi", "a")
	req.Header.Add("X-Multi", "b")
	req.Header["X-Empty"] = []string{}

	masked := collectRequestHeaders(req, mask)
	if masked["Authorization"] != masking.DefaultPlaceholder {
		t.Fatalf("敏感头应脱敏: %v", masked["Authorization"])
	}
	if masked["Content-Type"] != "application/json" {
		t.Fatalf("普通头应保留: %v", masked["Content-Type"])
	}
	if masked["X-Multi"] != "a,b" {
		t.Fatalf("多值头应合并: %v", masked["X-Multi"])
	}
	if _, ok := masked["X-Empty"]; ok {
		t.Fatal("空值头应跳过")
	}

	var nilMask *masking.Service
	raw := collectRequestHeaders(req, nilMask)
	if raw["Authorization"] != "Bearer abc" {
		t.Fatalf("未注入服务时应保留原值: %v", raw["Authorization"])
	}
	if collectRequestHeaders(nil, mask) != nil {
		t.Fatal("nil 请求应返回 nil")
	}
}

// TestMiddlewareStrategyEndToEnd 验证请求体/响应体/query 按策略输出
func TestMiddlewareStrategyEndToEnd(t *testing.T) {
	mask, err := masking.New(masking.Config{Rules: []masking.RuleConfig{
		{Keys: []string{"password"}, Strategy: masking.StrategyRedact},
		{Keys: []string{"phone"}, Strategy: masking.StrategyPhone},
	}})
	if err != nil {
		t.Fatalf("masking.New: %v", err)
	}

	rec := &recorderService{}
	cfg := Config{MiddlewareEnabled: true, RecordRequestBody: true, RecordResponseBody: true, MaskSensitive: true}
	engine := newTestEngine(t, cfg, rec, mask)
	engine.POST("/biz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"phone": "13812345678"})
	})

	req := authenticatedRequest(http.MethodPost, "/biz?contact_phone=13987654321", `{"password":"p@ss","phone":"13812345678"}`)
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(httptest.NewRecorder(), req)

	record := waitForRecords(t, rec, 1)[0]
	if !strings.Contains(record.RequestBody, `"password":"***"`) || strings.Contains(record.RequestBody, "p@ss") {
		t.Fatalf("请求体 redact 不符: %q", record.RequestBody)
	}
	if !strings.Contains(record.RequestBody, `"phone":"138****5678"`) {
		t.Fatalf("请求体 phone 策略不符: %q", record.RequestBody)
	}
	if !strings.Contains(record.ResponseBody, `"phone":"138****5678"`) {
		t.Fatalf("响应体 phone 策略不符: %q", record.ResponseBody)
	}
	// Encode 会把 * 转义为 %2A，先解码再断言
	query, err := url.QueryUnescape(record.Query)
	if err != nil {
		t.Fatalf("query 解码失败: %v", err)
	}
	if !strings.Contains(query, "139****4321") || strings.Contains(query, "13987654321") {
		t.Fatalf("query phone 策略不符: %q", query)
	}
}
