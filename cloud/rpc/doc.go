// Package rpc 提供跨服务的 HTTP RPC 传输能力。
//
// 职责边界：
//   - 负责端点解析（EndpointResolver）、POST JSON、响应反序列化、传输级重试、熔断与链路头透传；
//   - 不解析业务信封，不区分分页；业务成功由必填的 BizHookFunc 判定，
//     BizHookFunc 返回 (ok, error)，ok=false 且 error 为 nil 时返回 ErrBizRejected；
//   - 纯传输使用 NoopBizHook；传输成功 = HTTP 200 且响应反序列化成功
//     （RespBody == nil 时跳过反序列化），业务失败不影响重试与熔断。
//
// @author zwei
package rpc
