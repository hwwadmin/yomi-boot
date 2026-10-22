// Package traceid 提供请求链路追踪 ID 能力
//
// @author zwei
package traceid

import (
	"context"
	"net/http"
	"strings"

	"gitee.com/hq_project/yomi-boot/core/shared"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// spanContextKey context 中存储 SpanContext 的键类型
type spanContextKey struct{}

// SpanContext 链路元数据：trace 与 span 成对存储、语义独立
type SpanContext struct {
	TraceID      string
	SpanID       string
	ParentSpanID string // 可选；入站 traceparent 的 span 可作为 parent
}

// ContextWithSpanContext 将完整链路元数据写入上下文
func ContextWithSpanContext(ctx context.Context, sc SpanContext) context.Context {
	ctx = orBackground(ctx)
	sc.TraceID = strings.TrimSpace(sc.TraceID)
	sc.SpanID = strings.TrimSpace(sc.SpanID)
	sc.ParentSpanID = strings.TrimSpace(sc.ParentSpanID)
	if sc.TraceID == "" {
		return ctx
	}
	if sc.SpanID == "" {
		sc.SpanID = newSpanID()
	}
	return context.WithValue(ctx, spanContextKey{}, sc)
}

// ContextWithTraceID 将链路 ID 写入上下文，并确保带有 root span_id（兼容旧调用）
func ContextWithTraceID(ctx context.Context, traceID string) context.Context {
	return ContextWithSpanContext(ctx, SpanContext{TraceID: traceID, SpanID: newSpanID()})
}

// SpanFromContext 从上下文读取链路元数据；不存在时返回零值
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

// FromContext 从上下文读取链路 ID，不存在时返回空字符串
func FromContext(ctx context.Context) string {
	return SpanFromContext(ctx).TraceID
}

// GetTraceID 从请求上下文读取当前链路 ID
func GetTraceID(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	return FromContext(c.Request.Context())
}

// SetTraceID 在出站 HTTP 请求头中写入链路上下文（traceparent / X-Trace-ID / X-Request-ID）
// 使用 ctx 中当前的 span_id，保证与日志一致；缺失 span 时临时生成（不回写 ctx）
func SetTraceID(ctx context.Context, req *http.Request) {
	if req == nil {
		return
	}
	sc := SpanFromContext(ctx)
	if sc.TraceID == "" {
		return
	}
	spanID := sc.SpanID
	if spanID == "" {
		spanID = newSpanID()
	}
	writeTraceHeadersTo(req.Header, sc.TraceID, spanID)
}

// resolveTraceID 按规范优先级解析链路 ID
func resolveTraceID(r *http.Request) string {
	if r == nil {
		return newTraceID()
	}
	if id, _ := parseTraceparent(r.Header.Get(shared.TraceParentHeader)); id != "" {
		return id
	}
	if id := strings.TrimSpace(r.Header.Get(shared.TraceIDHeader)); id != "" {
		return id
	}
	return newTraceID()
}

// parseTraceparent 从 W3C traceparent 解析 trace-id 与 parent span-id；非法返回空
func parseTraceparent(raw string) (traceID, parentSpanID string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	parts := strings.Split(raw, "-")
	if len(parts) < 4 {
		return "", ""
	}
	traceID = strings.ToLower(parts[1])
	if len(traceID) != 32 || !isHex(traceID) {
		return "", ""
	}
	// 全 0 非法
	if traceID == "00000000000000000000000000000000" {
		return "", ""
	}
	parentSpanID = strings.ToLower(parts[2])
	if len(parentSpanID) != 16 || !isHex(parentSpanID) || parentSpanID == "0000000000000000" {
		parentSpanID = ""
	}
	return traceID, parentSpanID
}

// newTraceID 生成 32 hex 链路 ID（UUID 去横线）
func newTraceID() string {
	return strings.ReplaceAll(uuid.New().String(), "-", "")
}

// orBackground 将 nil context 归一化为 Background，避免派生 context 时 panic
func orBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
