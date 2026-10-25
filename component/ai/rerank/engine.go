// 门面接口与 Engine 定义。
//
// @author zwei

package rerank

import "context"

// Reranker 精重排门面接口；实现须并发安全
type Reranker interface {
	// Rerank 按查询对候选文本打相关性分
	//
	// 返回与 req.Docs 等长的分数列表，与输入下标一一对应，越大越相关，口径由实现定义；
	// 不得修改 req；实现未提供用量时 RerankResponse.Tokens 置 0。
	Rerank(ctx context.Context, req RerankRequest) (*RerankResponse, error)
}

// Engine 组合 Reranker 的通用精重排门面；负责入参校验、实现输出兜底校验、排序与截断，
// 不认识业务元数据语义；构造后不可变，可并发调用，请勿复制值
type Engine struct {
	reranker Reranker
}
