// @author zwei

package rpc

import (
	"context"
	"time"
)

// EndpointResolver 把服务名和逻辑端点解析成完整 URL
// Consul 场景使用 consul.NewConsulRPCResolver(client)
// 本地/测试场景使用 MemoryEndpointResolver
// 实现必须尊重 ctx；发现阶段超时由实现自行控制，InvokeRequest.Timeout 只约束 HTTP 请求
type EndpointResolver interface {
	ResolveHTTPBase(ctx context.Context, serviceName, pathSuffix string) (string, error)
}

// InvokeRequest 一次 RPC 调用
type InvokeRequest struct {
	Endpoint         string        // 逻辑端点
	ReqBody          any           // 请求对象，只读，可能含敏感信息
	RespBody         any           // 响应对象；nil 时不读取响应体
	BizHook          BizHookFunc   // 业务成功判定，必填
	Timeout          time.Duration // 单次尝试超时，<=0 时默认 3s；不是整个 Invoke 的总时长，总时长由调用方 ctx 控制
	MaxAttempts      int           // 最大尝试次数，<=0 时默认 1
	MaxResponseBytes int64         // 响应体上限，<=0 时默认 4 MiB
}

// BizHookContext 业务判定上下文
// Endpoint 是调用方传入的逻辑端点，不是解析后的 URL
// hook 只读 ReqBody/RespBody
type BizHookContext struct {
	Endpoint string // 逻辑端点
	ReqBody  any    // 请求对象，只读
	RespBody any    // 已反序列化的响应对象，只读
}

// BizHookFunc 在传输成功后判定业务是否成功（HTTP 200 且反序列化成功）
// error 非 nil 时原样返回；ok=false 且 error=nil 时返回 ErrBizRejected
// hook 必填；纯传输使用 NoopBizHook
// hook 可能并发调用，需保证线程安全
type BizHookFunc func(ctx context.Context, call BizHookContext) (ok bool, err error)

// NoopBizHook 跳过业务判定，始终返回成功
func NoopBizHook(context.Context, BizHookContext) (bool, error) {
	return true, nil
}

// RpcService 跨服务 RPC 传输接口
// 负责发送请求、反序列化响应、重试和熔断；业务成功由 req.BizHook 判定
type RpcService interface {
	// Invoke 调用远程端点，把响应体反序列化到 req.RespBody
	// 每次尝试使用 req.Timeout；普通调用按 MaxAttempts 重试；半开探测只尝试一次
	// 整个 Invoke 的总时长由调用方 ctx 控制，req.Timeout 不是总预算
	// BizHook 必填；纯传输使用 NoopBizHook
	Invoke(ctx context.Context, req InvokeRequest) error
}
