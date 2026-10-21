// Package shared 包提供系统级常量定义
//
// @author zwei
package shared

// 分页常量
const (
	// DefaultPage 是默认页码
	DefaultPage = 1
	// DefaultPageSize 是默认每页条数
	DefaultPageSize = 10
	// MaxPageSize 是每页最大条数
	MaxPageSize = 100
)

// 链路追踪常量
const (
	// TraceID 日志字段名（ELK / 规范 snake_case）
	TraceID = "trace_id"
	// SpanID 日志字段名：当前 span（与 trace_id 成对、语义独立）
	SpanID = "span_id"
	// ParentSpanID 日志字段名：父 span（可选）
	ParentSpanID = "parent_span_id"
	// TraceParentHeader W3C traceparent
	TraceParentHeader = "traceparent"
	// TraceIDHeader 跨服务透传主 Header
	TraceIDHeader = "X-Trace-ID"
	// RequestIDHeader 请求 ID Header
	RequestIDHeader = "X-Request-ID"
	// MissingTraceID 上下文中不存在链路 ID 时的占位值
	MissingTraceID = "-1"
	// MissingSpanID 上下文中不存在 span ID 时的占位值
	MissingSpanID = "-1"
)
