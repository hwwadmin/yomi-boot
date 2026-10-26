// 路径①文档入库：切块 → 向量化 → 记录 → 覆盖写入。
//
// @author zwei

package rag

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/ai/chunking"
	"gitee.com/hq_project/yomi-boot/component/ai/embedding"
	"gitee.com/hq_project/yomi-boot/component/ai/vectordb"
	"gitee.com/hq_project/yomi-boot/component/core/bc/random"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// chunkedDocument 切块后的文档
type chunkedDocument struct {
	doc     Document
	chunks  []string
	version string // version 是本次写入的文档版本
}

// Index 实现 Engine：切块、向量化并写入指定集合；集合不存在时自动创建
func (e *engine) Index(ctx context.Context, collection string, req IndexRequest) (*IndexResult, error) {
	ctx = orBackground(ctx)
	name, err := normalizeCollection(collection)
	if err != nil {
		return nil, err
	}
	docs, err := prepareDocuments(req.Documents)
	if err != nil {
		return nil, err
	}
	chunker := req.Chunker
	if chunker == nil {
		chunker = e.chunker
	}
	// 先切块并校验，全部通过后再创建集合与写入
	prepared, err := chunkDocuments(chunker, docs)
	if err != nil {
		return nil, err
	}
	if err := e.store.EnsureCollection(ctx, name); err != nil {
		return nil, err
	}
	result := &IndexResult{}
	for _, p := range prepared {
		tokens, err := e.writeDocument(ctx, name, p)
		if err != nil {
			return nil, err
		}
		result.Documents++
		result.Chunks += len(p.chunks)
		result.Tokens += tokens
	}
	return result, nil
}

// chunkDocuments 对每个文档切块、校验契约并生成写入版本
func chunkDocuments(chunker chunking.Chunker, docs []Document) ([]chunkedDocument, error) {
	prepared := make([]chunkedDocument, 0, len(docs))
	for _, doc := range docs {
		chunks, err := chunker.Chunk(doc.Content)
		if err != nil {
			return nil, err
		}
		// 校验块契约：空白块丢弃，超长块返回 chunking.ErrInvalidArgument
		chunks, err = chunking.NormalizeChunks(chunks)
		if err != nil {
			return nil, err
		}
		if len(chunks) > maxChunksPerDoc {
			return nil, errors.Sentinel(ErrInvalidArgument)
		}
		version, err := random.Hex(16)
		if err != nil {
			return nil, wrapSentinel(ErrBadResponse, err)
		}
		prepared = append(prepared, chunkedDocument{doc: doc, chunks: chunks, version: version})
	}
	return prepared, nil
}

// writeDocument 向量化单篇文档的块并覆盖写入；同一文档的替换由 Store 串行
func (e *engine) writeDocument(ctx context.Context, collection string, p chunkedDocument) (int, error) {
	resp, err := e.embedder.Embed(ctx, embedding.Request{Inputs: p.chunks})
	if err != nil {
		return 0, err
	}
	if resp == nil || len(resp.Vectors) != len(p.chunks) {
		return 0, wrapSentinel(ErrBadResponse, errors.New("ai/rag 向量化返回条数与输入不一致"))
	}
	records := make([]vectordb.Record, len(p.chunks))
	for i, content := range p.chunks {
		records[i] = encodeChunkRecord(p.doc, i, len(p.chunks), p.version, content, resp.Vectors[i])
	}
	if err := e.store.ReplaceByFilter(ctx, collection, map[string]string{MetadataKeyDocumentID: p.doc.ID}, records); err != nil {
		return 0, err
	}
	tokens := 0
	if resp.Usage != nil {
		tokens = resp.Usage.TotalTokens
	}
	return tokens, nil
}
