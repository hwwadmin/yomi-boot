// Package rpc 提供跨服务的 HTTP RPC 传输能力。
//
// 负责端点解析、POST JSON、响应反序列化、传输级重试、熔断和链路头透传。
// 业务信封由调用方的 BizHookFunc 判定：
// 返回 (ok, error)，ok=false 且 error=nil 时返回 ErrBizRejected。
// 纯传输使用 NoopBizHook；传输成功指 HTTP 200 且响应反序列化成功
// （RespBody 为 nil 时跳过反序列化），重试和熔断只看传输结果。
// InvokeRequest.Timeout 是单次尝试的超时，不是整个 Invoke 的总时长；
// 重试次数由 MaxAttempts 决定，总时长由调用方 ctx 控制。
// Timeout 只约束 HTTP 请求；发现阶段由 EndpointResolver 自行控制超时。
// 没有调用方 deadline 时，最坏总时长约为 MaxAttempts*Timeout + 退避 + Retry-After。
//
// @author zwei
package rpc
