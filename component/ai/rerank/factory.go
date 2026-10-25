// New 校验配置并装配 Engine。
//
// @author zwei

package rerank

// New 构造 Engine；Reranker 为空时使用内置 PassThrough；不访问网络
func New(cfg Config) (*Engine, error) {
	reranker := cfg.Reranker
	if reranker == nil {
		reranker = NewPassThrough()
	}
	return &Engine{reranker: reranker}, nil
}
