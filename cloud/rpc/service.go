// @author zwei

package rpc

import "context"

// BizHookContext 业务判定所需的调用上下文
// Endpoint 是调用方传入的逻辑端点，不是解析后的完整 URL
// hook 只能读取本结构，不得修改 ReqBody/RespBody
type BizHookContext struct {
	Endpoint string // Endpoint 是调用方传入的逻辑端点
	ReqBody  any    // ReqBody 是调用方传入的请求对象（只读；可能含敏感信息）
	RespBody any    // RespBody 是已反序列化的响应对象（只读）
}

// BizHookFunc 在传输成功（HTTP 200 且反序列化成功）后判断业务是否成功
// ok=true 表示业务成功；ok=false 表示业务失败
// 返回 error 时 Invoke 原样返回；ok=false 且 error 为 nil 时返回 ErrBizRejected
// hook 必须非 nil；纯传输使用 NoopBizHook
// hook 可能被并发调用，需自行保证线程安全
type BizHookFunc func(ctx context.Context, call BizHookContext) (ok bool, err error)

// NoopBizHook 只做传输、不判断业务；ok 永远为 true
func NoopBizHook(context.Context, BizHookContext) (bool, error) {
	return true, nil
}

// RpcService 跨服务 RPC 传输门面
// 只负责发送请求、反序列化响应与传输级韧性；业务成功由 bizHook 判定
type RpcService interface {
	// Invoke 调用远程端点，将响应体反序列化到 respBody
	// bizHook 不能为 nil；纯传输使用 NoopBizHook
	Invoke(ctx context.Context, endpoint string, reqBody any, respBody any, bizHook BizHookFunc) error
}
