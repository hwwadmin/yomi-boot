// 定义文档与块模型。
//
// @author zwei

package rag

// Document 待入库的原始文档
type Document struct {
	ID       string            // ID 是文档唯一标识；trim 后非空，最长 120 字节（Milvus VARCHAR 按字节计）
	Content  string            // Content 是原始文本；trim 后非空
	Source   string            // Source 是来源（URL / 路径等），可选；写入 rag_source
	Metadata map[string]string // Metadata 是自定义元数据；键 trim 后非空且不得使用保留键
}

// Chunk 一个可检索的文本块
type Chunk struct {
	ID          string            // ID 是块主键，格式为 "文档ID#序号"
	DocumentID  string            // DocumentID 是所属文档 ID
	Index       int               // Index 是块在文档内的序号，从 0 开始
	Content     string            // Content 是块文本
	Source      string            // Source 是文档来源，可能为空
	Metadata    map[string]string // Metadata 是文档自定义元数据副本；不含保留键
	Score       float32           // Score 是检索/融合相关性分，越大越相似；口径见 ai/retrieval
	RerankScore float32           // RerankScore 是精重排相关性分，越大越相关；口径由 Reranker 实现定义，不可跨实现横比；PassThrough 恒 0
}
