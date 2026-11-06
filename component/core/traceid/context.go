package traceid

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type spanContextKey struct{}

// SpanContext 是内部链路。写出日志和 HTTP 头时只使用 Traceparent()。
type SpanContext struct {
	TraceID      string // 32 位十六进制追踪号
	SpanID       string // 16 位十六进制，本跳 span
	ParentSpanID string // 入站调用方的 span；可空
	Flags        string // 2 位十六进制
	Tracestate   string // 原样转发；可空
}

// Traceparent 拼出本跳的 W3C traceparent。字段不合法时返回空字符串。
func (sc SpanContext) Traceparent() string {
	traceID := strings.ToLower(strings.TrimSpace(sc.TraceID))
	spanID := strings.ToLower(strings.TrimSpace(sc.SpanID))
	flags := strings.ToLower(strings.TrimSpace(sc.Flags))
	if flags == "" {
		flags = traceFlagsRecorded
	}
	if !validTraceID(traceID) || !validSpanID(spanID) || !validFlags(flags) {
		return ""
	}
	return traceparentVersion + "-" + traceID + "-" + spanID + "-" + flags
}

// ContextWithSpanContext 写入一条已经规范的链路。追踪号或本跳 span 不合法时不写入。
func ContextWithSpanContext(ctx context.Context, sc SpanContext) context.Context {
	ctx = orBackground(ctx)
	sc.TraceID = strings.ToLower(strings.TrimSpace(sc.TraceID))
	sc.SpanID = strings.ToLower(strings.TrimSpace(sc.SpanID))
	sc.ParentSpanID = strings.ToLower(strings.TrimSpace(sc.ParentSpanID))
	sc.Flags = strings.ToLower(strings.TrimSpace(sc.Flags))
	sc.Tracestate = strings.TrimSpace(sc.Tracestate)
	if !validFlags(sc.Flags) {
		sc.Flags = traceFlagsRecorded
	}
	if sc.Traceparent() == "" {
		return ctx
	}
	if sc.ParentSpanID != "" && !validSpanID(sc.ParentSpanID) {
		sc.ParentSpanID = ""
	}
	return context.WithValue(ctx, spanContextKey{}, sc)
}

// ContextWithTraceparent 把一条本服务发出的 traceparent 恢复进上下文。
// 头里的 parent-id 视为本跳 span。非法时不写入。
func ContextWithTraceparent(ctx context.Context, header string) context.Context {
	parsed, ok := parseTraceparent(header)
	if !ok {
		return orBackground(ctx)
	}
	return ContextWithSpanContext(ctx, SpanContext{
		TraceID: parsed.traceID,
		SpanID:  parsed.spanID,
		Flags:   parsed.flags,
	})
}

// SpanFromContext 读取内部链路；不存在时返回零值。
func SpanFromContext(ctx context.Context) SpanContext {
	if ctx == nil {
		return SpanContext{}
	}
	sc, ok := ctx.Value(spanContextKey{}).(SpanContext)
	if !ok {
		return SpanContext{}
	}
	return sc
}

// FromContext 返回本跳 traceparent。没有链路时返回空字符串。
func FromContext(ctx context.Context) string {
	return SpanFromContext(ctx).Traceparent()
}

// GetTraceparent 从 Gin 请求上下文读取本跳 traceparent。
func GetTraceparent(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	return FromContext(c.Request.Context())
}

// SetTraceparent 给出站请求写入 traceparent。有 tracestate 时原样带上。没有链路时不写头。
func SetTraceparent(ctx context.Context, req *http.Request) {
	if req == nil {
		return
	}
	writeTraceparent(req.Header, SpanFromContext(ctx))
}

func orBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
