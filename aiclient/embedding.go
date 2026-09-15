// Package aiclient 定义向量化门面与请求响应结构。
//
// @author zwei
package aiclient

import (
	"context"
	"strings"

	"gitee.com/hq_project/yomi-boot/errors"
)

// EmbeddingClient 协议无关的同步向量化门面；同一实例可并发调用
type EmbeddingClient interface {
	// Embed 将文本转为向量
	//
	// 入参 Inputs 至少 1 条；任一条 trim 后空串非法。维度空则回退 EmbeddingConfig.Dimensions。
	// 一次调用整批成功或整包失败。超时或取消返回上下文错误（可包装）。
	Embed(ctx context.Context, req EmbeddingRequest) (*EmbeddingResponse, error)
}

// EmbeddingRequest 向量化入参；模型由 EmbeddingConfig.Model 锁定
type EmbeddingRequest struct {
	Inputs     []string // Inputs 至少 1 条；trim 后不得为空串
	Dimensions *int     // Dimensions 可选；空则回退配置；若设置必须 >0
}

// EmbeddingResponse 向量化完整结果
type EmbeddingResponse struct {
	Model   string      // Model 是实际上游模型名
	Vectors [][]float64 // Vectors 与 Inputs 按下标对齐
	Usage   *TokenUsage // Usage 可选；仅填 Prompt/Total；全 0 时为 nil
}

// validateEmbeddingRequest 校验输入列表与请求侧维度；非法返回 ErrInvalidArgument
func validateEmbeddingRequest(req EmbeddingRequest) error {
	if len(req.Inputs) == 0 {
		return errors.Sentinel(ErrInvalidArgument)
	}
	for _, s := range req.Inputs {
		if strings.TrimSpace(s) == "" {
			return errors.Sentinel(ErrInvalidArgument)
		}
	}
	if req.Dimensions != nil && *req.Dimensions <= 0 {
		return errors.Sentinel(ErrInvalidArgument)
	}
	return nil
}

// resolveDimensions 请求非空则用之，否则回退配置；皆空则不传上游
func resolveDimensions(req, def *int) *int {
	if req != nil {
		return req
	}
	return def
}
