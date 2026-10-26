// 构造配置。
//
// @author zwei

package rag

import (
	"gitee.com/hq_project/yomi-boot/component/ai/chunking"
	"gitee.com/hq_project/yomi-boot/component/ai/embedding"
	"gitee.com/hq_project/yomi-boot/component/ai/generation"
	"gitee.com/hq_project/yomi-boot/component/ai/rerank"
	"gitee.com/hq_project/yomi-boot/component/ai/retrieval"
	"gitee.com/hq_project/yomi-boot/component/ai/vectordb"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

const (
	maxDocIDLen     = 120 // 文档 ID 最大字节数，为 "文档ID#序号" 预留 8 字节
	maxChunksPerDoc = 10000
)

// Config 创建 Engine 的构造配置：5 个流程门面与向量库全部注入且必填
type Config struct {
	Chunker   chunking.Chunker     // Chunker 是文档入库的切块节点；可经 IndexRequest.Chunker 按调用覆盖
	Embedder  embedding.Client     // Embedder 是文档入库的向量化节点
	Retriever retrieval.Retriever  // Retriever 是检索节点；与 Store 使用同一向量库，查询向量化与 Embedder 同模型同维度
	Reranker  rerank.Reranker      // Reranker 是精重排节点；未接入重排模型时用 rerank.NewPassThrough
	Generator generation.Generator // Generator 是提示词拼接节点；未定制模板时用 generation.NewBasic
	Store     vectordb.Store       // Store 是向量库（写入与域过滤辅助查询）；向量维度与向量化输出一致
}

// validate 校验构造配置
func (c Config) validate() error {
	if c.Chunker == nil || c.Embedder == nil || c.Retriever == nil ||
		c.Reranker == nil || c.Generator == nil || c.Store == nil {
		return errors.Sentinel(ErrInvalidArgument)
	}
	return nil
}

// normalizeCollection 修剪并校验集合名；空串或非法命名返回 ErrInvalidArgument
func normalizeCollection(name string) (string, error) {
	n, err := vectordb.NormalizeCollectionName(name)
	if err != nil {
		return "", errors.Sentinel(ErrInvalidArgument)
	}
	return n, nil
}
