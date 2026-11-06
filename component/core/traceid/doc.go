// Package traceid 提供 W3C traceparent：Gin 中间件、上下文存取与出站写入。
//
// 请求只认合法的 traceparent。不合规则本地新开一条。日志、响应和出站请求使用同一串。
// 入站 tracestate 原样转发，不解析。
//
// @author zwei
package traceid
