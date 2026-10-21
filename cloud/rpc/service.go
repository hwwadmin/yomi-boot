// @author zwei

package rpc

import "context"

// BizHookContext 业务判定所需的调用上下文
// Endpoint 是调用方传入的逻辑端点，不是解析后的完整 URL
type BizHookContext struct {
	Endpoint string // Endpoint 是调用方传入的逻辑端点
	ReqBody  any    // ReqBody 是调用方传入的请求对象（只读）
	RespBody any    // RespBody 是已反序列化的响应对象（只读）
}

// BizHookFunc 在传输成功（HTTP 200 且反序列化成功）后判断业务是否成功
// 返回 nil 表示业务成功；返回 error 表示业务失败，Invoke 原样返回该 error
// hook 不参与重试、不改变熔断状态，耗时不计入熔断延迟
type BizHookFunc func(ctx context.Context, call BizHookContext) error

// RpcService 跨服务 RPC 传输门面
// 只负责发送请求、反序列化响应与传输级韧性；业务信封由调用方通过 bizHook 解析
type RpcService interface {
	// Invoke 调用远程端点，将响应体反序列化到 respBody
	// bizHook 为 nil 时只做传输；非 nil 时在传输成功后判定业务是否成功
	Invoke(ctx context.Context, endpoint string, reqBody any, respBody any, bizHook BizHookFunc) error
}
