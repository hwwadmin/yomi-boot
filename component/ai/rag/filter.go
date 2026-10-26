// 文档域过滤器：作为 retrieval.HitFilter 实现，按文档版本与块总数丢弃旧块。
//
// @author zwei

package rag

import (
	"context"
	"strconv"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/ai/retrieval"
	"gitee.com/hq_project/yomi-boot/component/ai/vectordb"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/logger"
)

// chunkFilter 文档域过滤器；同一实例可并发调用
type chunkFilter struct {
	store vectordb.Store
}

var _ retrieval.HitFilter = (*chunkFilter)(nil)

// newChunkFilter 创建文档域过滤器
func newChunkFilter(store vectordb.Store) *chunkFilter {
	return &chunkFilter{store: store}
}

// Filter 实现 retrieval.HitFilter：忽略缺少块文本的命中，按文档当前版本与块总数丢弃旧块
func (f *chunkFilter) Filter(ctx context.Context, collection string, hits []vectordb.Hit) ([]vectordb.Hit, error) {
	candidates := make([]vectordb.Hit, 0, len(hits))
	for _, hit := range hits {
		if hasContent(hit) {
			candidates = append(candidates, hit)
		}
	}
	states, err := f.loadDocumentStates(ctx, collection, candidates)
	if err != nil {
		return nil, err
	}
	out := make([]vectordb.Hit, 0, len(candidates))
	for _, hit := range candidates {
		if isCurrentChunk(hit, states) {
			out = append(out, hit)
		}
	}
	return out, nil
}

// loadDocumentStates 读取命中文档当前版本与块总数；辅助查询失败时降级为不过滤
func (f *chunkFilter) loadDocumentStates(ctx context.Context, collection string, hits []vectordb.Hit) (map[string]documentState, error) {
	ids := make([]string, 0, len(hits))
	seen := make(map[string]struct{}, len(hits))
	for _, hit := range hits {
		docID := strings.TrimSpace(hit.Metadata[MetadataKeyDocumentID])
		// 非本包写入的超长文档 ID 不参与块数查询，避免 Get 因主键超长整体失败
		if docID == "" || len(docID) > maxDocIDLen {
			continue
		}
		if _, ok := seen[docID]; ok {
			continue
		}
		seen[docID] = struct{}{}
		ids = append(ids, chunkID(docID, 0))
	}
	if len(ids) == 0 {
		return nil, nil
	}
	records, err := f.store.Get(ctx, collection, ids)
	if err != nil {
		// 取消/超时必须上抛，不能把 ctx 失效降级成正常结果
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		if errors.Is(err, vectordb.ErrNotFound) {
			return nil, err
		}
		// 其他辅助查询失败时降级为不过滤，避免检索整体失败
		logger.Warn(ctx, "ai/rag 读取文档版本失败，降级为不过滤旧块", "collection", collection, "error", err)
		return nil, nil
	}
	states := make(map[string]documentState, len(records))
	for _, record := range records {
		docID := strings.TrimSpace(record.Metadata[MetadataKeyDocumentID])
		if docID == "" {
			continue
		}
		total, err := strconv.Atoi(strings.TrimSpace(record.Metadata[MetadataKeyChunkCount]))
		if err != nil || total <= 0 {
			continue
		}
		states[docID] = documentState{
			total:   total,
			version: strings.TrimSpace(record.Metadata[MetadataKeyVersion]),
		}
	}
	return states, nil
}
