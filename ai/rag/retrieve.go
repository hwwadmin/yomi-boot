// 检索。
//
// @author zwei

package rag

import (
	"context"
	"math"
	"strconv"
	"strings"

	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/vectordb"
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
	out := make([]Chunk, 0, len(hits))
	for _, hit := range hits {
		if req.MinScore != 0 && hit.Score < req.MinScore {
			continue
		}
		content := strings.TrimSpace(hit.Metadata[MetadataKeyContent])
		if content == "" {
			continue
		}
		out = append(out, hitToChunk(hit, content))
	}
	return out, nil
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
