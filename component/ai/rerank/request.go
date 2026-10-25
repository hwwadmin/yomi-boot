// 定义重排请求与结果类型。
//
// @author zwei

package rerank

// MaxDocs 单次重排的候选文本上限，与主流重排服务对齐；超过请分批调用
const MaxDocs = 1000

// RerankRequest 精重排入参
type RerankRequest struct {
	Query string   // Query 是排序依据的问题；trim 后非空
	Docs  []string // Docs 是候选文本；1~MaxDocs 条，每条 trim 后非空；下标即结果回指的 Index
	TopN  int      // TopN 是返回的最大条数；0 表示全部返回；负数非法；超过 len(Docs) 时钳制
}

// RerankResponse Reranker 实现的输出
type RerankResponse struct {
	Scores []float32 // Scores 与 req.Docs 等长且一一对应；越大越相关；不得含 NaN/Inf
	Tokens int       // Tokens 是重排 token 用量合计；上游未提供时为 0
}

// RankedDoc 一条重排结果
type RankedDoc struct {
	Index int     // Index 在请求 Docs 中的下标；调用方据此回取原对象
	Score float32 // Score 相关性分，越大越相关；口径由 Reranker 实现定义，不可跨实现横比
}

// RerankResult 精重排结果
type RerankResult struct {
	Results []RankedDoc // Results 按分数降序；同分保持输入顺序（稳定排序）
	Tokens  int         // Tokens 是重排 token 用量合计；上游未提供时为 0
}
