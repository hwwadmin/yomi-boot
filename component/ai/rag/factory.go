// New 校验配置并装配 Engine。
//
// @author zwei

package rag

import "gitee.com/hq_project/yomi-boot/component/ai/chunking"

// New 校验配置并构造 Engine；不访问网络、不创建集合
func New(cfg Config) (*Engine, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	defaultChunker, err := chunking.New(chunking.Config{})
	if err != nil {
		return nil, err
	}
	return &Engine{
		embedder:       cfg.Embedder,
		store:          cfg.Store,
		defaultChunker: defaultChunker,
	}, nil
}
