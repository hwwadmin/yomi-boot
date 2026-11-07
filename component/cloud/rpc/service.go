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
	Timeout          time.Duration // 单次尝试超时，<=0 时默认 3s；不是整个 Invoke 的总时长，总时长由调用方 ctx 控制
	MaxAttempts      int           // 最大尝试次数，<=0 时默认 1
	MaxResponseBytes int64         // 响应体上限，<=0 时默认 4 MiB
}

// RpcService 跨服务 RPC 传输接口
// 负责发送请求、反序列化响应、重试和熔断。HTTP 200 且反序列化成功即返回 nil，业务成败由调用方看 RespBody
type RpcService interface {
	// Invoke 调用远程端点，把响应体反序列化到 req.RespBody
	// 每次尝试使用 req.Timeout；普通调用按 MaxAttempts 重试；半开探测只尝试一次
	// 整个 Invoke 的总时长由调用方 ctx 控制，req.Timeout 不是总预算
	Invoke(ctx context.Context, req InvokeRequest) error
}
