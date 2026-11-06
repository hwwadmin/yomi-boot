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

// 链路追踪常量。对外只有 W3C traceparent，日志字段与头使用同一个名字。
const (
	// Traceparent 是日志字段名，也是 HTTP 头名
	Traceparent = "traceparent"
	// Tracestate 是 W3C tracestate 头；只原样转发，不解析
	Tracestate = "tracestate"
)
