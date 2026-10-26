// 契约的可执行形式：入参校验、分数排序与结果回取，供 Reranker 实现与调用方复用。
//
// @author zwei

package rerank

import (
	"math"
	"sort"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// ValidateRequest 校验重排请求：Query 与每条候选 trim 后非空、Docs 1~MaxDocs 条、
// TopN 不为负；供 Reranker 实现自查入参契约，非法返回 ErrInvalidArgument
func ValidateRequest(req RerankRequest) error {
	if strings.TrimSpace(req.Query) == "" {
		return errors.Sentinel(ErrInvalidArgument)
	}
	if len(req.Docs) == 0 || len(req.Docs) > MaxDocs || req.TopN < 0 {
		return errors.Sentinel(ErrInvalidArgument)
	}
	for _, doc := range req.Docs {
		if strings.TrimSpace(doc) == "" {
			return errors.Sentinel(ErrInvalidArgument)
		}
	}
	return nil
}

// RankScores 将按输入顺序排列的分数整理为降序结果，同分保持输入顺序；
// 供按输入顺序打分的 Reranker 实现复用（如调上游重排服务拿到等长分数后转换）。
// topN 为 0 表示全部返回，负数返回 ErrInvalidArgument；分数含 NaN/Inf 返回 ErrBadResponse
func RankScores(scores []float32, topN int) ([]RankedDoc, error) {
	if topN < 0 {
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
	for _, score := range scores {
		if math.IsNaN(float64(score)) || math.IsInf(float64(score), 0) {
			return nil, errors.Sentinel(ErrBadResponse)
		}
	}
	ranked := make([]RankedDoc, len(scores))
	for i, score := range scores {
		ranked[i] = RankedDoc{Index: i, Score: score}
	}
	// 稳定排序：同分保持输入顺序；全 0 分因此等于保序
	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].Score > ranked[j].Score
	})
	if topN > 0 && topN < len(ranked) {
		// 三索引切片：调用方对结果追加不得越界覆盖被截断部分
		return ranked[:topN:topN], nil
	}
	return ranked, nil
}

// Scored 一条应用重排结果后的候选
type Scored[T any] struct {
	Item  T       // Item 是候选对象，按 RankedDoc.Index 从调用方切片回取
	Score float32 // Score 是重排相关性分，口径由 Reranker 实现定义，不可跨实现横比
}

// Apply 把重排结果应用到候选切片：按下标回取 Item，按重排顺序返回；
// 供调用方把 RerankResponse 映射回自有对象（契约的可执行形式）。
// resp 为 nil 或 Results 下标越界返回 ErrBadResponse；候选可为空（返回空切片）
func Apply[T any](items []T, resp *RerankResponse) ([]Scored[T], error) {
	if resp == nil {
		return nil, errors.Sentinel(ErrBadResponse)
	}
	out := make([]Scored[T], 0, len(resp.Results))
	for _, r := range resp.Results {
		if r.Index < 0 || r.Index >= len(items) {
			return nil, errors.Sentinel(ErrBadResponse)
		}
		out = append(out, Scored[T]{Item: items[r.Index], Score: r.Score})
	}
	return out, nil
}
