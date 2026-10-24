// Package traceid 提供请求链路追踪 ID 能力：W3C traceparent 解析、Gin 中间件与 context 存取
//
// 链路 ID 解析优先级：traceparent → X-Trace-ID → 本地生成。
//
// @author zwei
package traceid
