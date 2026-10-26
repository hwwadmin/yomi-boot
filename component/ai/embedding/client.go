// 定义向量化门面与请求响应结构。
//
// @author zwei

package embedding

import (
	"context"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// Client 协议无关的同步向量化门面；同一实例可并发调用
type Client interface {
	// Embed 将文本转为向量
	//
	// 入参 Inputs 至少 1 条；任一条 trim 后空串非法；Dimensions 必填且 >0。
	// 超过 Config.MaxBatch（默认 2048）时按该值分批，对外仍是一整批成功或整包失败。
	// 超时或取消返回上下文错误（可包装）。
	Embed(ctx context.Context, req Request) (*Response, error)
}

// Request 向量化入参；模型由 Config.Model 锁定
type Request struct {
	Inputs     []string // Inputs 至少 1 条；trim 后不得为空串；超过 MaxBatch（默认 2048）时内部自动分批
	Dimensions int      // Dimensions 是输出维度；必填且必须 >0
}

// Response 向量化完整结果
type Response struct {
	Model   string      // Model 是实际上游模型名
	Vectors [][]float64 // Vectors 与 Inputs 按下标对齐
	Usage   *TokenUsage // Usage 可选；仅填 Prompt/Total；全 0 时为 nil
}

// TokenUsage 向量化用量；无 completion 概念
type TokenUsage struct {
	PromptTokens int // PromptTokens 是输入令牌数
	TotalTokens  int // TotalTokens 是合计令牌数
}

// validateRequest 校验输入列表与请求侧维度；非法返回 ErrInvalidArgument
func validateRequest(req Request) error {
	if len(req.Inputs) == 0 {
		return errors.Sentinel(ErrInvalidArgument)
	}
	for _, s := range req.Inputs {
		if strings.TrimSpace(s) == "" {
			return errors.Sentinel(ErrInvalidArgument)
		}
	}
	if req.Dimensions <= 0 {
		return errors.Sentinel(ErrInvalidArgument)
	}
	return nil
}
