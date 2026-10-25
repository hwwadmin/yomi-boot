// 构造配置；本包不读 YAML。
//
// @author zwei

package rerank

// Config 创建 Engine 的构造配置；Reranker 为空时使用内置 PassThrough（保序返回 TOP-N）
type Config struct {
	Reranker Reranker // Reranker 精重排实现；为空则使用内置 PassThrough 占位
}
