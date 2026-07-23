package traceid

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"gitee.com/hq_project/yomi-boot/shared"

	"github.com/gin-gonic/gin"
)

// TestResolveTraceID_Priority 验证 traceparent 优先于 X-Trace-ID 解析链路 ID
func TestResolveTraceID_Priority(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(shared.TraceIDHeader, "x-trace-id-value")
	req.Header.Set(shared.TraceParentHeader, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")

	got := resolveTraceID(req)
	if got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("want traceparent id, got %q", got)
	}
}

// TestResolveTraceID_XTraceID 验证 X-Trace-ID 解析链路 ID
func TestResolveTraceID_XTraceID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(shared.TraceIDHeader, "my-trace-001")
	got := resolveTraceID(req)
	if got != "my-trace-001" {
		t.Fatalf("got %q", got)
	}
}

// TestResolveTraceID_GenerateWhenMissing 验证无入站头时自动生成 32 位 hex 链路 ID
func TestResolveTraceID_GenerateWhenMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	got := resolveTraceID(req)
	if len(got) != 32 {
		t.Fatalf("want 32-hex generated id, got %q", got)
	}
}

// TestMiddleware_WritesStandardHeadersAndSpanInCtx 验证中间件写入标准响应头并在 ctx 保存 span
func TestMiddleware_WritesStandardHeadersAndSpanInCtx(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware())
	var gotSC SpanContext
	r.GET("/ping", func(c *gin.Context) {
		gotSC = SpanFromContext(c.Request.Context())
		c.String(200, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set(shared.TraceParentHeader, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	r.ServeHTTP(w, req)

	if gotSC.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("ctx trace=%q", gotSC.TraceID)
	}
	if len(gotSC.SpanID) != 16 {
		t.Fatalf("ctx span=%q", gotSC.SpanID)
	}
	if gotSC.ParentSpanID != "00f067aa0ba902b7" {
		t.Fatalf("parent span=%q", gotSC.ParentSpanID)
	}
	if got := w.Header().Get(shared.TraceIDHeader); got != gotSC.TraceID {
		t.Fatalf("X-Trace-ID=%q want %q", got, gotSC.TraceID)
	}
	if got := w.Header().Get(shared.RequestIDHeader); got != gotSC.TraceID {
		t.Fatalf("X-Request-ID=%q", got)
	}
	tp := w.Header().Get(shared.TraceParentHeader)
	wantTP := formatTraceparent(gotSC.TraceID, gotSC.SpanID)
	if tp != wantTP {
		t.Fatalf("traceparent=%q want %q (must reuse ctx span)", tp, wantTP)
	}
}

// TestSetTraceID_OutboundHeadersReuseCtxSpan 验证出站请求头复用 ctx 中的 span_id
func TestSetTraceID_OutboundHeadersReuseCtxSpan(t *testing.T) {
	ctx := ContextWithSpanContext(context.TODO(), SpanContext{
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:  "aabbccddeeff0011",
	})
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api", nil)
	SetTraceID(ctx, req)
	if req.Header.Get(shared.TraceIDHeader) != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("X-Trace-ID=%q", req.Header.Get(shared.TraceIDHeader))
	}
	tp := req.Header.Get(shared.TraceParentHeader)
	if tp != "00-4bf92f3577b34da6a3ce929d0e0e4736-aabbccddeeff0011-01" {
		t.Fatalf("traceparent=%q", tp)
	}
	if req.Header.Get(shared.RequestIDHeader) == "" {
		t.Fatal("X-Request-ID empty")
	}
}

// TestContextWithTraceID_AlwaysHasRootSpan 验证 ContextWithTraceID 始终写入 root span_id
func TestContextWithTraceID_AlwaysHasRootSpan(t *testing.T) {
	ctx := ContextWithTraceID(context.TODO(), "abc123")
	sc := SpanFromContext(ctx)
	if sc.TraceID != "abc123" {
		t.Fatalf("trace=%q", sc.TraceID)
	}
	if len(sc.SpanID) != 16 {
		t.Fatalf("span=%q", sc.SpanID)
	}
}

// TestParseTraceparent_Invalid 验证非法 traceparent 解析返回空
func TestParseTraceparent_Invalid(t *testing.T) {
	if tid, _ := parseTraceparent(""); tid != "" {
		t.Fatal("empty")
	}
	if tid, _ := parseTraceparent("00-00000000000000000000000000000000-00f067aa0ba902b7-01"); tid != "" {
		t.Fatal("all-zero trace")
	}
}
