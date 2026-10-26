// 构造配置；本包不读 YAML。
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

// Config 创建 Engine 的构造配置；两条流程的门面节点与基础设施全部必填，
// 各节点的具体实现（切块策略、向量化、检索、重排、提示词拼接）由调用方装配
type Config struct {
	Chunker   chunking.Chunker     // Chunker 是路径①切块节点；必填；可经 IndexRequest.Chunker 按调用覆盖
	Embedder  embedding.Client     // Embedder 是路径①向量化节点；必填
	Retriever retrieval.Retriever  // Retriever 是路径②检索节点；必填；须与 Store 基于同一向量库，其查询向量化须与 Embedder 同模型同维度
	Reranker  rerank.Reranker      // Reranker 是路径②精重排节点；必填；未接入重排模型可用 rerank.NewPassThrough
	Generator generation.Generator // Generator 是路径②提示词拼接末节点；必填；未定制模板可用 generation.NewBasic
	Store     vectordb.Store       // Store 是向量库基础设施（写入与域过滤辅助查询）；必填；各集合 Dimension 必须与向量化输出一致
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
