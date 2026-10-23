// 检索。
//
// @author zwei

package rag

import (
	"context"
	"math"
	"strconv"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/ai/vectordb"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/logger"
)

// Retrieve 在指定集合中检索最相关的块；集合不存在或为空时返回空切片与 nil
func (e *Engine) Retrieve(ctx context.Context, collection string, req RetrieveRequest) ([]Chunk, error) {
	name, err := normalizeCollection(collection)
	if err != nil {
		return nil, err
	}
	return e.retrieve(orBackground(ctx), name, req)
}

// retrieve 执行检索
func (e *Engine) retrieve(ctx context.Context, collection string, req RetrieveRequest) ([]Chunk, error) {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
	if req.TopK <= 0 || req.TopK > vectordb.MaxTopK {
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
	if math.IsNaN(float64(req.MinScore)) || math.IsInf(float64(req.MinScore), 0) {
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
	if err := validateFilter(req.Filter); err != nil {
		return nil, err
	}
	// 集合不存在时不消耗向量化调用，严格按契约返回空结果
	has, err := e.store.HasCollection(ctx, collection)
	if err != nil {
		return nil, err
	}
	if !has {
		return []Chunk{}, nil
	}
	vectors, _, err := e.embed(ctx, []string{query})
	if err != nil {
		return nil, err
	}
	hits, err := e.store.Search(ctx, collection, vectordb.SearchQuery{
		Vector: toFloat32(vectors[0]),
		TopK:   req.TopK,
		Filter: req.Filter,
	})
	if err != nil {
		// 集合尚未建立等同于空检索
		if errors.Is(err, vectordb.ErrNotFound) {
			return []Chunk{}, nil
		}
		return nil, err
	}
	candidates := make([]vectordb.Hit, 0, len(hits))
	for _, hit := range hits {
		if req.MinScore != 0 && hit.Score < req.MinScore {
			continue
		}
		if strings.TrimSpace(hit.Metadata[MetadataKeyContent]) == "" {
			continue
		}
		candidates = append(candidates, hit)
	}
	// 读取命中文档当前版本与块总数，过滤更新窗口内尚未删除的旧块
	states, err := e.currentChunkStates(ctx, collection, candidates)
	if err != nil {
		if errors.Is(err, vectordb.ErrNotFound) {
			return []Chunk{}, nil
		}
		return nil, err
	}
	out := make([]Chunk, 0, len(candidates))
	for _, hit := range candidates {
		if state, ok := states[strings.TrimSpace(hit.Metadata[MetadataKeyDocumentID])]; ok {
			index, err := strconv.Atoi(strings.TrimSpace(hit.Metadata[MetadataKeyChunkIndex]))
			if err == nil {
				version := strings.TrimSpace(hit.Metadata[MetadataKeyVersion])
				if state.version != "" && version != state.version {
					continue
				}
				if index >= state.total {
					continue
				}
			}
		}
		out = append(out, hitToChunk(hit, strings.TrimSpace(hit.Metadata[MetadataKeyContent])))
	}
	return out, nil
}

// chunkState 是一次写入的版本与块总数
type chunkState struct {
	total   int
	version string
}

// currentChunkStates 读取命中文档当前版本与块总数；辅助查询失败时降级为不过滤
func (e *Engine) currentChunkStates(ctx context.Context, collection string, hits []vectordb.Hit) (map[string]chunkState, error) {
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
	states := make(map[string]chunkState, len(ids))
	for start := 0; start < len(ids); start += idBatchSize {
		end := start + idBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		records, err := e.store.Get(ctx, collection, ids[start:end])
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
		for _, record := range records {
			docID := strings.TrimSpace(record.Metadata[MetadataKeyDocumentID])
			if docID == "" {
				continue
			}
			total, err := strconv.Atoi(strings.TrimSpace(record.Metadata[MetadataKeyChunkCount]))
			if err != nil || total <= 0 {
				continue
			}
			states[docID] = chunkState{
				total:   total,
				version: strings.TrimSpace(record.Metadata[MetadataKeyVersion]),
			}
		}
	}
	return states, nil
}

// hitToChunk 将向量库命中转为块，并剥离保留元数据键
func hitToChunk(hit vectordb.Hit, content string) Chunk {
	c := Chunk{
		ID:         hit.ID,
		DocumentID: strings.TrimSpace(hit.Metadata[MetadataKeyDocumentID]),
		Content:    content,
		Source:     strings.TrimSpace(hit.Metadata[MetadataKeySource]),
		Score:      hit.Score,
	}
	if index, err := strconv.Atoi(hit.Metadata[MetadataKeyChunkIndex]); err == nil && index >= 0 {
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
