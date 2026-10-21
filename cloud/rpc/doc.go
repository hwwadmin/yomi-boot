// Package rpc 提供跨服务的 HTTP RPC 传输能力。
//
// 负责端点解析、POST JSON、响应反序列化、传输级重试、熔断和链路头透传。
// 业务信封由调用方的 BizHookFunc 判定：
// 返回 (ok, error)，ok=false 且 error=nil 时返回 ErrBizRejected。
// 纯传输使用 NoopBizHook；传输成功指 HTTP 200 且响应反序列化成功
// （RespBody 为 nil 时跳过反序列化），重试和熔断只看传输结果。
//
// @author zwei
package rpc
