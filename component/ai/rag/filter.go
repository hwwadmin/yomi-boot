// 文档域过滤器装配：过滤实现由 ai/retrieval 提供，本包只声明元数据协议。
//
// @author zwei

package rag

import (
	"gitee.com/hq_project/yomi-boot/component/ai/retrieval"
	"gitee.com/hq_project/yomi-boot/component/ai/vectordb"
)

// newChunkFilter 装配文档域过滤器：旧块丢弃与缺块文本命中忽略
// 由 retrieval.NewDocumentFilter 承担，本包只提供保留键与首块主键推导
func newChunkFilter(store vectordb.Store) (retrieval.HitFilter, error) {
	return retrieval.NewDocumentFilter(store, retrieval.DocumentFilterConfig{
		DocumentKey: MetadataKeyDocumentID,
		IndexKey:    MetadataKeyChunkIndex,
		CountKey:    MetadataKeyChunkCount,
		VersionKey:  MetadataKeyVersion,
		ContentKey:  MetadataKeyContent,
		FirstChunkID: func(docID string) string {
			// 非本包写入的超长文档 ID 不参与状态查询，避免 Get 因主键超长整体失败
			if len(docID) > maxDocIDLen {
				return ""
			}
			return chunkID(docID, 0)
		},
	})
}
