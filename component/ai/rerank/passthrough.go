// 内置空实现：不打分，仅保序截断。
//
// @author zwei

package rerank

import "context"

// passThrough 空实现：Score 恒为 0，同分保序即等于保持输入顺序
type passThrough struct{}

var _ Reranker = passThrough{}

// NewPassThrough 返回空实现；用于业务尚未接入具体重排模型时的默认占位，
// 接入后替换为自有 Reranker 实现
func NewPassThrough() Reranker {
	return passThrough{}
}

// Rerank 实现 Reranker；保序返回 TOP-N，Score 恒为 0
func (passThrough) Rerank(ctx context.Context, req RerankRequest) (*RerankResponse, error) {
	if err := ValidateRequest(req); err != nil {
		return nil, err
	}
	topN := len(req.Docs)
	if req.TopN > 0 && req.TopN < topN {
		topN = req.TopN
	}
	results := make([]RankedDoc, topN)
	for i := range results {
		results[i] = RankedDoc{Index: i}
	}
	return &RerankResponse{Results: results}, nil
}
