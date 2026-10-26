// 块元数据协议：保留键、块主键与双向编解码。
//
// @author zwei

package rag

import (
	"strconv"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/ai/vectordb"
)

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

// chunkID 拼接块主键；文档 ID 最长 120，块序号最大 4 位，总长不超过 128
func chunkID(docID string, index int) string {
	return docID + "#" + strconv.Itoa(index)
}

// encodeChunkMetadata 合并文档自定义元数据与保留键；total 是块总数，version 是本次写入版本
func encodeChunkMetadata(doc Document, index, total int, version, content string) map[string]string {
	md := make(map[string]string, len(doc.Metadata)+6)
	for k, v := range doc.Metadata {
		md[k] = v
	}
	md[MetadataKeyDocumentID] = doc.ID
	md[MetadataKeyChunkIndex] = strconv.Itoa(index)
	md[MetadataKeyChunkCount] = strconv.Itoa(total)
	md[MetadataKeyVersion] = version
	md[MetadataKeySource] = doc.Source
	md[MetadataKeyContent] = content
	return md
}

// hasContent 判断命中是否含块文本；非本包写入的命中不含
func hasContent(hit vectordb.Hit) bool {
	return strings.TrimSpace(hit.Metadata[MetadataKeyContent]) != ""
}

// decodeHit 将含块文本的命中转为块，并剥离保留元数据键
func decodeHit(hit vectordb.Hit) Chunk {
	c := Chunk{
		ID:         hit.ID,
		DocumentID: strings.TrimSpace(hit.Metadata[MetadataKeyDocumentID]),
		Content:    strings.TrimSpace(hit.Metadata[MetadataKeyContent]),
		Source:     strings.TrimSpace(hit.Metadata[MetadataKeySource]),
		Score:      hit.Score,
	}
	if index, err := strconv.Atoi(strings.TrimSpace(hit.Metadata[MetadataKeyChunkIndex])); err == nil && index >= 0 {
		c.Index = index
	}
	md := make(map[string]string, len(hit.Metadata))
	for rawKey, v := range hit.Metadata {
		key := strings.TrimSpace(rawKey)
		if key == "" || isReservedMetadataKey(key) {
			continue
		}
		md[key] = v
	}
	if len(md) == 0 {
		md = nil
	}
	c.Metadata = md
	return c
}

// isReservedMetadataKey 判断是否为本包保留的元数据键
func isReservedMetadataKey(key string) bool {
	switch key {
	case MetadataKeyDocumentID, MetadataKeyChunkIndex, MetadataKeySource, MetadataKeyContent, MetadataKeyChunkCount, MetadataKeyVersion:
		return true
	default:
		return false
	}
}
