package traceid

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/hwwadmin/yomi-boot/component/core/shared"

	"github.com/gin-gonic/gin"
)

const (
	traceparentVersion = "00"
	traceFlagsRecorded = "01"
)

// Middleware 为每个请求建立本跳 span，并把 traceparent 写进响应头。
// 入站头合法时沿用追踪号和 flags，另建本跳 span；否则新开一条链路。
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		rawTP, rawTS := "", ""
		if c.Request != nil {
			rawTP = c.Request.Header.Get(shared.Traceparent)
			rawTS = c.Request.Header.Get(shared.Tracestate)
		}
		sc := continueOrNew(rawTP, rawTS)
		if c.Request != nil {
			c.Request = c.Request.WithContext(ContextWithSpanContext(c.Request.Context(), sc))
		}
		writeTraceparent(c.Writer.Header(), sc)
		c.Next()
	}
}

// continueOrNew 接受一条入站 traceparent，或新开一条根链路。
func continueOrNew(rawTP, rawTS string) SpanContext {
	parsed, ok := parseTraceparent(rawTP)
	if !ok {
		return newRootSpan()
	}
	sc := SpanContext{
		TraceID:      parsed.traceID,
		SpanID:       newSpanID(),
		ParentSpanID: parsed.spanID,
		Flags:        parsed.flags,
	}
	if ts := strings.TrimSpace(rawTS); ts != "" {
		sc.Tracestate = ts
	}
	return sc
}

// newRootSpan 生成一条新的已采样链路。
func newRootSpan() SpanContext {
	return SpanContext{
		TraceID: newTraceID(),
		SpanID:  newSpanID(),
		Flags:   traceFlagsRecorded,
	}
}

// writeTraceparent 写入 traceparent；有 tracestate 时原样带上。
func writeTraceparent(h http.Header, sc SpanContext) {
	if h == nil {
		return
	}
	tp := sc.Traceparent()
	if tp == "" {
		return
	}
	h.Set(shared.Traceparent, tp)
	if ts := strings.TrimSpace(sc.Tracestate); ts != "" {
		h.Set(shared.Tracestate, ts)
	}
}

type parsedHeader struct {
	traceID string
	spanID  string
	flags   string
}

// parseTraceparent 解析 W3C traceparent。只接受版本 00，且追踪号、span、flags 都合法。
func parseTraceparent(raw string) (parsedHeader, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return parsedHeader{}, false
	}
	parts := strings.Split(raw, "-")
	if len(parts) < 4 || parts[0] != traceparentVersion {
		return parsedHeader{}, false
	}
	traceID := strings.ToLower(parts[1])
	spanID := strings.ToLower(parts[2])
	flags := strings.ToLower(parts[3])
	if !validTraceID(traceID) || !validSpanID(spanID) || !validFlags(flags) {
		return parsedHeader{}, false
	}
	return parsedHeader{traceID: traceID, spanID: spanID, flags: flags}, true
}

func validTraceID(id string) bool {
	return len(id) == 32 && isHex(id) && id != "00000000000000000000000000000000"
}

func validSpanID(id string) bool {
	return len(id) == 16 && isHex(id) && id != "0000000000000000"
}

func validFlags(flags string) bool {
	return len(flags) == 2 && isHex(flags)
}

func newTraceID() string {
	var b [16]byte
	return randomHex(b[:])
}

func newSpanID() string {
	var b [8]byte
	return randomHex(b[:])
}

func randomHex(b []byte) string {
	for {
		if _, err := rand.Read(b); err != nil {
			return ""
		}
		id := hex.EncodeToString(b)
		if !allZero(b) {
			return id
		}
	}
}

func allZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return s != ""
}
