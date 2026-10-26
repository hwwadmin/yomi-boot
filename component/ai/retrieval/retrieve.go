// 检索执行与策略路由。
//
// @author zwei

package retrieval

import (
	"context"
	"math"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/ai/embedding"
	"gitee.com/hq_project/yomi-boot/component/ai/vectordb"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// Retrieve 在指定集合中检索最相关条目；集合不存在或为空时返回空切片与 nil
func (e *Engine) Retrieve(ctx context.Context, collection string, req RetrieveRequest) ([]vectordb.Hit, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
	strategy, err := normalizeStrategy(req.Strategy)
	if err != nil {
		return nil, err
	}
	if req.TopK <= 0 || req.TopK > vectordb.MaxTopK {
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
	if req.Dimensions <= 0 {
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
	if math.IsNaN(float64(req.MinScore)) || math.IsInf(float64(req.MinScore), 0) {
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
	if err := validateFilter(req.Filter); err != nil {
		return nil, err
	}
	// 集合不存在时不消耗向量化调用，严格按契约返回空结果
	has, err := e.store.HasCollection(ctx, collection)
	if err != nil {
		return nil, err
	}
	if !has {
		return []vectordb.Hit{}, nil
	}
	vector, err := e.embedQuery(ctx, query, req.Dimensions)
	if err != nil {
		return nil, err
	}
	// hybrid 把查询原文同时交给 BM25 一路；vector 只走向量
	searchText := ""
	if strategy == StrategyHybrid {
		searchText = query
	}
	hits, err := e.store.Search(ctx, collection, vectordb.SearchQuery{
		Vector: vector,
		Text:   searchText,
		TopK:   req.TopK,
		Filter: req.Filter,
	})
	if err != nil {
		// 集合尚未建立等同于空检索
		if errors.Is(err, vectordb.ErrNotFound) {
			return []vectordb.Hit{}, nil
		}
		return nil, err
	}
	hits = filterByScore(hits, req.MinScore)
	return applyPostFilter(ctx, collection, req.PostFilter, hits)
}

// filterByScore 按 MinScore 过滤命中；0 表示不设阈值
func filterByScore(hits []vectordb.Hit, minScore float32) []vectordb.Hit {
	if minScore == 0 {
		return hits
	}
	out := make([]vectordb.Hit, 0, len(hits))
	for _, hit := range hits {
		if hit.Score < minScore {
			continue
		}
		out = append(out, hit)
	}
	return out
}

// applyPostFilter 应用后置过滤器；集合消失等同于空检索，其他错误原样上抛
func applyPostFilter(ctx context.Context, collection string, filter HitFilter, hits []vectordb.Hit) ([]vectordb.Hit, error) {
	if filter == nil || len(hits) == 0 {
		return hits, nil
	}
	out, err := filter.Filter(ctx, collection, hits)
	if err != nil {
		if errors.Is(err, vectordb.ErrNotFound) {
			return []vectordb.Hit{}, nil
		}
		return nil, err
	}
	return out, nil
}

// normalizeStrategy 空策略回退 hybrid，并校验白名单
func normalizeStrategy(strategy RetrievalStrategy) (RetrievalStrategy, error) {
	if strategy == "" {
		return StrategyHybrid, nil
	}
	switch strategy {
	case StrategyHybrid, StrategyVector:
		return strategy, nil
	default:
		return "", errors.Sentinel(ErrInvalidArgument)
	}
}

// embedQuery 按请求维度向量化查询文本；集合维度不符由 vectordb 校验兜底
func (e *Engine) embedQuery(ctx context.Context, query string, dimensions int) ([]float32, error) {
	resp, err := e.embedder.Embed(ctx, embedding.Request{
		Inputs:     []string{query},
		Dimensions: dimensions,
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Vectors) != 1 || len(resp.Vectors[0]) == 0 {
		return nil, errors.Sentinel(ErrBadResponse)
	}
	return toFloat32(resp.Vectors[0]), nil
}

// toFloat32 将向量化输出转为向量库所需的 float32
func toFloat32(vec []float64) []float32 {
	out := make([]float32, len(vec))
	for i, v := range vec {
		out[i] = float32(v)
	}
	return out
}
