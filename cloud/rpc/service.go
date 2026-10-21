// Package rpc 提供跨服务 RPC 调用门面，屏蔽底层通信协议差异
//
// @author zwei
package rpc

import (
	"context"
)

// RpcService 跨服务 RPC 调用门面接口
// 定义统一的远程调用契约，HTTP / gRPC 等具体协议实现各自适配
// traceID 通过 context 传递，各实现按协议写入对应的传输机制
type RpcService interface {
	// Invoke 调用远程服务端点，reqBody 作为请求体，响应反序列化到 respData
	Invoke(ctx context.Context, endpoint string, reqBody any, respData any) error

	// InvokePage 调用远程服务分页端点，反序列化响应并返回总条数
	InvokePage(ctx context.Context, endpoint string, reqBody any, respData any) (int64, error)
}
