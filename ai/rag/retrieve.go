// 检索。
//
// @author zwei

package rag

import (
	"context"
	"math"
	"strconv"
	"strings"

	"gitee.com/hq_project/yomi-boot/ai/vectordb"
	"gitee.com/hq_project/yomi-boot/errors"
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
	// 读取命中文档当前块总数，过滤索引更新窗口内尚未删除的旧块
	counts, err := e.currentChunkCounts(ctx, collection, hits)
	if err != nil {
		if errors.Is(err, vectordb.ErrNotFound) {
			return []Chunk{}, nil
		}
		return nil, err
	}
	out := make([]Chunk, 0, len(hits))
	for _, hit := range hits {
		if req.MinScore != 0 && hit.Score < req.MinScore {
			continue
		}
		if count, ok := counts[strings.TrimSpace(hit.Metadata[MetadataKeyDocumentID])]; ok {
			index, err := strconv.Atoi(strings.TrimSpace(hit.Metadata[MetadataKeyChunkIndex]))
			if err == nil && index >= count {
				continue
			}
		}
		content := strings.TrimSpace(hit.Metadata[MetadataKeyContent])
		if content == "" {
			continue
		}
		out = append(out, hitToChunk(hit, content))
	}
	return out, nil
}

// currentChunkCounts 读取命中文档当前版本的块总数；用于过滤更新窗口内的旧块
func (e *Engine) currentChunkCounts(ctx context.Context, collection string, hits []vectordb.Hit) (map[string]int, error) {
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
	records, err := e.store.Get(ctx, collection, ids)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(records))
	for _, record := range records {
		docID := strings.TrimSpace(record.Metadata[MetadataKeyDocumentID])
		if docID == "" {
			continue
		}
		total, err := strconv.Atoi(strings.TrimSpace(record.Metadata[MetadataKeyChunkCount]))
		if err != nil || total <= 0 {
			continue
		}
		counts[docID] = total
	}
	return counts, nil
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
