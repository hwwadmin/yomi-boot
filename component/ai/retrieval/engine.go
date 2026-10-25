// 门面接口与 Engine 定义。
//
// @author zwei

package retrieval

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/ai/embedding"
	"gitee.com/hq_project/yomi-boot/component/ai/vectordb"
)

// Retriever 检索门面接口；实现须并发安全
type Retriever interface {
	// Retrieve 在指定集合中检索最相关条目；集合不存在或为空返回空切片与 nil
	Retrieve(ctx context.Context, collection string, req RetrieveRequest) ([]vectordb.Hit, error)
}

// Engine 组合 Embedder 与 Store 的通用检索门面；同一实例可并发服务多个集合；构造后不可变，请勿复制值
type Engine struct {
	embedder embedding.Client
	store    vectordb.Store
}

var _ Retriever = (*Engine)(nil)

// New 校验配置并构造 Engine；不访问网络、不创建集合
func New(cfg Config) (*Engine, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &Engine{embedder: cfg.Embedder, store: cfg.Store}, nil
}
