// Package rpc 提供跨服务的 HTTP RPC 传输能力。
//
// 职责边界：
//   - 负责服务发现、POST JSON、响应反序列化、传输级重试、熔断与链路头透传；
//   - 不解析业务信封，不区分分页；业务成功由调用方通过 BizHookFunc 判定；
//   - 传输成功 = HTTP 200 且响应反序列化成功，业务失败不影响重试与熔断。
//
// @author zwei
package rpc
