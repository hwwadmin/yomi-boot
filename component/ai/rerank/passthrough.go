// 内置空实现：不打分，仅保序截断。
//
// @author zwei

package rerank

import "context"

// passThrough 空实现：Score 恒为 0，稳定排序后等于保持输入顺序
type passThrough struct{}

var _ Reranker = passThrough{}

// NewPassThrough 返回空实现；用于业务尚未接入具体重排模型时的默认占位，
// 接入后经 Config.Reranker 注入自有实现
func NewPassThrough() Reranker {
	return passThrough{}
}

// Rerank 实现 Reranker；返回与候选等长的全 0 分数
func (passThrough) Rerank(ctx context.Context, req RerankRequest) (*RerankResponse, error) {
	return &RerankResponse{Scores: make([]float32, len(req.Docs))}, nil
}
