// 定义文档、块与请求响应类型。
//
// @author zwei

package rag

// 保留元数据键；由本包写入，Document.Metadata 不得使用
const (
	// MetadataKeyDocumentID 文档 ID
	MetadataKeyDocumentID = "rag_doc_id"
	// MetadataKeyChunkIndex 块在文档内的序号，从 0 开始
	MetadataKeyChunkIndex = "rag_chunk_index"
	// MetadataKeySource 文档来源
	MetadataKeySource = "rag_source"
	// MetadataKeyContent 块文本
	MetadataKeyContent = "rag_content"
	// MetadataKeyChunkCount 文档当前版本的块总数；用于检索侧过滤更新窗口内的旧块
	MetadataKeyChunkCount = "rag_chunk_count"
	// MetadataKeyVersion 文档当前写入版本；检索侧据此丢弃旧版本块
	MetadataKeyVersion = "rag_version"
)

// Document 待入库的原始文档
type Document struct {
	ID       string            // ID 是文档唯一标识；trim 后非空，最长 120 字节（Milvus VARCHAR 按字节计）
	Content  string            // Content 是原始文本；trim 后非空
	Source   string            // Source 是来源（URL / 路径等），可选；写入 rag_source
	Metadata map[string]string // Metadata 是自定义元数据；键 trim 后非空且不得使用保留键
}

// Chunk 一个可检索的文本块
type Chunk struct {
	ID         string            // ID 是块主键，格式为 "文档ID#序号"
	DocumentID string            // DocumentID 是所属文档 ID
	Index      int               // Index 是块在文档内的序号，从 0 开始
	Content    string            // Content 是块文本
	Source     string            // Source 是文档来源，可能为空
	Metadata   map[string]string // Metadata 是文档自定义元数据副本；不含保留键
	Score      float32           // Score 是相似度，越大越相似；仅检索结果有值
}

// IndexRequest 文档入库入参
type IndexRequest struct {
	Documents []Document // Documents 至少 1 条；同一请求内 ID 不得重复
	Splitter  Splitter   // Splitter 可选；空则使用内置默认 recursive（800 字 / 100 重叠）
}

// IndexResult 文档入库结果
type IndexResult struct {
	Documents int // Documents 是写入的文档数
	Chunks    int // Chunks 是切分并写入的块数
	Tokens    int // Tokens 是向量化用量合计；上游未提供时为 0
}

// RetrieveRequest 检索入参
type RetrieveRequest struct {
	Query    string            // Query 是检索问题；trim 后非空
	TopK     int               // TopK 是向向量库请求的最大条数；阈值/空块/旧块过滤后可能更少；必须 >0 且不超过 vectordb.MaxTopK
	MinScore float32           // MinScore 是最低相似度；0 表示不设阈值；NaN 非法
	Filter   map[string]string // Filter 可选；元数据等值 AND，可用保留键过滤
}
