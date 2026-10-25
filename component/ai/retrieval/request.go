// 定义检索请求与策略类型。
//
// @author zwei

package retrieval

// RetrievalStrategy 检索策略标识；空则 hybrid
type RetrievalStrategy string

const (
	// StrategyHybrid 向量 + BM25 混合检索（RRF 融合）；默认策略
	StrategyHybrid RetrievalStrategy = "hybrid"
	// StrategyVector 纯向量检索
	StrategyVector RetrievalStrategy = "vector"
)

// RetrieveRequest 检索入参
type RetrieveRequest struct {
	Query    string            // Query 是检索问题；trim 后非空；hybrid 下同时作为 BM25 检索文本
	Strategy RetrievalStrategy // Strategy 是检索策略；空则 hybrid；仅 hybrid / vector
	TopK     int               // TopK 是向向量库请求的最大条数；阈值过滤后可能更少；必须 >0 且不超过 vectordb.MaxTopK
	MinScore float32           // MinScore 是最低分；0 表示不设阈值；NaN/Inf 非法；hybrid 的 Score 是 RRF 融合分（量级约 0 ~ 2/61），与 vector 的 Score 不可横比
	Filter   map[string]string // Filter 可选；元数据等值 AND
}
