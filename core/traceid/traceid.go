// Package traceid 提供请求链路追踪 ID 能力
//
// @author zwei
package traceid

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"gitee.com/hq_project/yomi-boot/core/shared"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Middleware 返回请求链路追踪中间件。
// 解析优先级：traceparent → X-Trace-ID → 本地生成。
// 入口建立 root span_id（写入 ctx），响应 Header 复用该 span，与日志一致。
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID, parentSpanID := "", ""
		if c.Request != nil {
			if tid, ps := parseTraceparent(c.Request.Header.Get(shared.TraceParentHeader)); tid != "" {
				traceID, parentSpanID = tid, ps
			} else if id := strings.TrimSpace(c.Request.Header.Get(shared.TraceIDHeader)); id != "" {
				traceID = id
			}
		}
		if traceID == "" {
			traceID = newTraceID()
		}
		sc := SpanContext{
			TraceID:      traceID,
			SpanID:       newSpanID(),
			ParentSpanID: parentSpanID,
		}
		c.Request = c.Request.WithContext(ContextWithSpanContext(c.Request.Context(), sc))
		writeTraceHeadersTo(c.Writer.Header(), sc.TraceID, sc.SpanID)
		c.Next()
	}
}

// writeTraceHeadersTo 向 Header 写入 traceparent / X-Trace-ID / X-Request-ID
func writeTraceHeadersTo(h http.Header, traceID, spanID string) {
	if h == nil || strings.TrimSpace(traceID) == "" {
		return
	}
	h.Set(shared.TraceIDHeader, traceID)
	h.Set(shared.RequestIDHeader, traceID)
	if spanID == "" {
		spanID = newSpanID()
	}
	h.Set(shared.TraceParentHeader, formatTraceparent(traceID, spanID))
}

// formatTraceparent 生成 W3C traceparent：00-{trace}-{span}-01
func formatTraceparent(traceID, spanID string) string {
	return "00-" + normalizeTraceID(traceID) + "-" + normalizeSpanID(spanID) + "-01"
}

// normalizeTraceID 规范为 32 位小写 hex；非标准形态则用 SHA1-UUID 派生
func normalizeTraceID(id string) string {
	id = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(id)), "-", "")
	if len(id) == 32 && isHex(id) {
		return id
	}
	u := uuid.NewSHA1(uuid.NameSpaceOID, []byte(id))
	return strings.ReplaceAll(u.String(), "-", "")
}

// normalizeSpanID 规范为 16 位小写 hex；非法则重新生成
func normalizeSpanID(id string) string {
	id = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(id)), "-", "")
	if len(id) >= 16 && isHex(id[:16]) {
		return id[:16]
	}
	return newSpanID()
}

// newSpanID 生成 16 hex 的 span id
func newSpanID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// isHex 判断字符串是否全为十六进制字符
func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
