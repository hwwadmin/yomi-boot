// 文档域过滤器：声明 rag 的元数据协议，装配 ai/retrieval 的文档版本过滤器。
//
// @author zwei

package rag

import (
	"gitee.com/hq_project/yomi-boot/component/ai/retrieval"
	"gitee.com/hq_project/yomi-boot/component/ai/vectordb"
)

// newChunkFilter 装配文档域过滤器：保留键与首块主键由本包提供，
// 旧块丢弃与缺块文本命中忽略由 retrieval.NewDocumentFilter 执行
func newChunkFilter(store vectordb.Store) (retrieval.HitFilter, error) {
	return retrieval.NewDocumentFilter(store, retrieval.DocumentFilterConfig{
		DocumentKey: MetadataKeyDocumentID,
		IndexKey:    MetadataKeyChunkIndex,
		CountKey:    MetadataKeyChunkCount,
		VersionKey:  MetadataKeyVersion,
		ContentKey:  MetadataKeyContent,
		FirstChunkID: func(docID string) string {
			// 超长文档 ID 不参与状态查询，避免 Get 因主键超长整体失败
			if len(docID) > maxDocIDLen {
				return ""
			}
			return chunkID(docID, 0)
		},
	})
}
