// New 校验配置并装配 Engine。
//
// @author zwei

package rag

// New 校验配置并构造 Engine；不访问网络、不创建集合
func New(cfg Config) (*Engine, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &Engine{
		chunker:   cfg.Chunker,
		embedder:  cfg.Embedder,
		retriever: cfg.Retriever,
		reranker:  cfg.Reranker,
		generator: cfg.Generator,
		store:     cfg.Store,
	}, nil
}
