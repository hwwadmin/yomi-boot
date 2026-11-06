package traceid

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hwwadmin/yomi-boot/component/core/shared"

	"github.com/gin-gonic/gin"
)

const sampleTP = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

func TestMiddleware_ContinuesTraceparent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware())
	var got SpanContext
	r.GET("/ping", func(c *gin.Context) {
		got = SpanFromContext(c.Request.Context())
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set(shared.Traceparent, sampleTP)
	req.Header.Set(shared.Tracestate, "vendor=opaque")
	req.Header.Set("X-Trace-ID", "order-42")
	r.ServeHTTP(w, req)

	if got.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("trace=%q", got.TraceID)
	}
	if got.ParentSpanID != "00f067aa0ba902b7" {
		t.Fatalf("parent=%q", got.ParentSpanID)
	}
	if got.SpanID == got.ParentSpanID || len(got.SpanID) != 16 {
		t.Fatalf("本跳 span=%q", got.SpanID)
	}
	if got.Flags != "01" || got.Tracestate != "vendor=opaque" {
		t.Fatalf("flags=%q tracestate=%q", got.Flags, got.Tracestate)
	}
	want := got.Traceparent()
	if got := w.Header().Get(shared.Traceparent); got != want {
		t.Fatalf("响应 traceparent=%q，期望 %q", got, want)
	}
	if got := w.Header().Get(shared.Tracestate); got != "vendor=opaque" {
		t.Fatalf("响应 tracestate=%q", got)
	}
	if w.Header().Get("X-Trace-ID") != "" || w.Header().Get("X-Request-ID") != "" {
		t.Fatalf("不应再写自定义追踪头: %v", w.Header())
	}
	if strings.Contains(want, "order-42") {
		t.Fatalf("非标准头不应进入 traceparent: %s", want)
	}
}

func TestMiddleware_InvalidTraceparentStartsNew(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware())
	var got string
	r.GET("/ping", func(c *gin.Context) {
		got = FromContext(c.Request.Context())
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set(shared.Traceparent, "00-zzzz-00f067aa0ba902b7-01")
	req.Header.Set(shared.Tracestate, "vendor=drop")
	r.ServeHTTP(w, req)

	if got == "" || !strings.HasPrefix(got, "00-") || !strings.HasSuffix(got, "-01") {
		t.Fatalf("应新开一条 traceparent，实际 %q", got)
	}
	if strings.Contains(got, "zzzz") {
		t.Fatalf("非法头不应被沿用: %s", got)
	}
	if w.Header().Get(shared.Tracestate) != "" {
		t.Fatal("非法 traceparent 时不应转发 tracestate")
	}
	if w.Header().Get(shared.Traceparent) != got {
		t.Fatalf("响应头 %q 与上下文 %q 不一致", w.Header().Get(shared.Traceparent), got)
	}
}

func TestSetTraceparent_ReusesContext(t *testing.T) {
	ctx := ContextWithTraceparent(context.Background(), sampleTP)
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api", nil)
	SetTraceparent(ctx, req)
	if got := req.Header.Get(shared.Traceparent); got != sampleTP {
		t.Fatalf("traceparent=%q", got)
	}
	if req.Header.Get("X-Trace-ID") != "" || req.Header.Get("X-Request-ID") != "" {
		t.Fatal("出站不应写自定义追踪头")
	}
}

func TestSetTraceparent_ForwardsTracestate(t *testing.T) {
	ctx := ContextWithSpanContext(context.Background(), SpanContext{
		TraceID:    "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:     "00f067aa0ba902b7",
		Flags:      "00",
		Tracestate: "vendor=opaque",
	})
	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	SetTraceparent(ctx, req)
	if got := req.Header.Get(shared.Traceparent); got != "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00" {
		t.Fatalf("traceparent=%q", got)
	}
	if got := req.Header.Get(shared.Tracestate); got != "vendor=opaque" {
		t.Fatalf("tracestate=%q", got)
	}
}

func TestContextWithTraceparent_RejectsInvalid(t *testing.T) {
	ctx := ContextWithTraceparent(context.Background(), "order-42")
	if FromContext(ctx) != "" {
		t.Fatalf("非法头不应写入上下文: %q", FromContext(ctx))
	}
	if _, ok := parseTraceparent("00-00000000000000000000000000000000-00f067aa0ba902b7-01"); ok {
		t.Fatal("全 0 追踪号应拒绝")
	}
}

func TestParseTraceparent_Version00KeepsExtraFields(t *testing.T) {
	raw := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01-extra"
	parsed, ok := parseTraceparent(raw)
	if !ok {
		t.Fatal("版本 00 尾部多出来的字段应忽略")
	}
	if parsed.traceID != "4bf92f3577b34da6a3ce929d0e0e4736" || parsed.spanID != "00f067aa0ba902b7" || parsed.flags != "01" {
		t.Fatalf("parsed = %+v", parsed)
	}
	if _, ok := parseTraceparent("01-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"); ok {
		t.Fatal("非 00 版本应拒绝")
	}
}

func TestFromContext_EmptyWithoutSpan(t *testing.T) {
	if got := FromContext(context.Background()); got != "" {
		t.Fatalf("没有链路时应为空，实际 %q", got)
	}
}
