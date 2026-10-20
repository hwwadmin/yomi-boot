// Package rag New 校验配置并装配 Engine。
//
// @author zwei
package rag

// New 校验配置并构造 Engine；不访问网络、不创建集合
func New(cfg Config) (*Engine, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	collection, err := normalizeCollection(cfg.Collection)
	if err != nil {
		return nil, err
	}
	defaultSplitter, err := NewSplitter(SplitConfig{})
	if err != nil {
		return nil, err
	}
	return &Engine{
		embedder:        cfg.Embedder,
		store:           cfg.Store,
		collection:      collection,
		defaultSplitter: defaultSplitter,
	}, nil
}
