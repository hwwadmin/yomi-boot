// 门面接口定义。
//
// @author zwei

package rerank

import "context"

// Reranker 精重排门面接口；实现须并发安全
type Reranker interface {
	// Rerank 按查询对候选文本重排
	//
	// 入参契约：Query 与每条候选 trim 后非空；Docs 1~MaxDocs 条；TopN 为 0 表示
	// 全部返回，负数返回 ErrInvalidArgument。输出契约：Results 按相关性降序
	// （同分保持输入顺序），Index 回指 req.Docs 下标，TopN > 0 时不超过 TopN 条；
	// 不得修改 req；未提供用量时 RerankResponse.Tokens 置 0。
	// 实现可经 ValidateRequest 与 RankScores 复用契约校验与分数排序。
	Rerank(ctx context.Context, req RerankRequest) (*RerankResponse, error)
}
