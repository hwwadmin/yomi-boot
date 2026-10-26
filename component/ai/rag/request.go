// 定义两条流程的请求与响应类型。
//
// @author zwei

package rag

import (
	"gitee.com/hq_project/yomi-boot/component/ai/chunking"
	"gitee.com/hq_project/yomi-boot/component/ai/retrieval"
)

// IndexRequest 文档入库入参
type IndexRequest struct {
	Documents []Document       // Documents 至少 1 条；同一请求内 ID 不得重复
	Chunker   chunking.Chunker // Chunker 可选；空则使用 Config.Chunker
}

// IndexResult 文档入库结果
type IndexResult struct {
	Documents int // Documents 是写入的文档数
	Chunks    int // Chunks 是写入的块数
	Tokens    int // Tokens 是向量化用量合计；上游未提供时为 0
}

// RetrieveRequest 文档检索增强入参
type RetrieveRequest struct {
	retrieval.RetrieveRequest     // 检索段入参（Query/Strategy/TopK/MinScore/Filter）透传 ai/retrieval；Query 同时作为重排依据与拼接问题
	RerankTopN                int // RerankTopN 是重排后保留的最大条数；0 表示全部保留；负数非法
}

// RetrieveResult 文档检索增强输出
type RetrieveResult struct {
	Prompt string  // Prompt 是拼接后的完整输入提示词；直接作为大模型输入
	Chunks []Chunk // Chunks 是重排后的文档块；顺序即投喂拼接的顺序，供引用标注
}
