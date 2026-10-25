// 重排执行与契约兜底。
//
// @author zwei

package rerank

import (
	"context"
	"math"
	"sort"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// Rerank 按查询对候选文本重排，返回按分数降序的结果；TopN 大于 0 时截断
//
// 入参非法返回 ErrInvalidArgument；实现输出违反契约（缺失、长度不等、含 NaN/Inf）
// 返回 ErrBadResponse；实现自身的错误原样透传。
func (e *Engine) Rerank(ctx context.Context, req RerankRequest) (RerankResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(req.Query) == "" {
		return RerankResult{}, errors.Sentinel(ErrInvalidArgument)
	}
	if len(req.Docs) == 0 || len(req.Docs) > MaxDocs || req.TopN < 0 {
		return RerankResult{}, errors.Sentinel(ErrInvalidArgument)
	}
	for _, doc := range req.Docs {
		if strings.TrimSpace(doc) == "" {
			return RerankResult{}, errors.Sentinel(ErrInvalidArgument)
		}
	}
	resp, err := e.reranker.Rerank(ctx, req)
	if err != nil {
		return RerankResult{}, err
	}
	if resp == nil || len(resp.Scores) != len(req.Docs) {
		return RerankResult{}, errors.Sentinel(ErrBadResponse)
	}
	for _, score := range resp.Scores {
		if math.IsNaN(float64(score)) || math.IsInf(float64(score), 0) {
			return RerankResult{}, errors.Sentinel(ErrBadResponse)
		}
	}
	ranked := make([]RankedDoc, len(req.Docs))
	for i, score := range resp.Scores {
		ranked[i] = RankedDoc{Index: i, Score: score}
	}
	// 稳定排序：同分保持输入顺序；PassThrough 全 0 分因此等于保序
	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].Score > ranked[j].Score
	})
	topN := len(ranked)
	if req.TopN > 0 && req.TopN < topN {
		topN = req.TopN
	}
	tokens := resp.Tokens
	if tokens < 0 {
		tokens = 0
	}
	// 三索引切片：调用方对 Results 追加不得越界覆盖被截断部分
	return RerankResult{Results: ranked[:topN:topN], Tokens: tokens}, nil
}
