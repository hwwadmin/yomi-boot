// Package rag 文档入库与删除。
//
// @author zwei
package rag

import (
	"context"
	"strconv"
	"strings"

	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/vectordb"
)

// preparedDocument 切分后的文档
type preparedDocument struct {
	doc    Document
	chunks []string
}

// idBatchSize 是按 ID 读写时的单批上限，避免一次请求携带过多主键
const idBatchSize = 500

// Index 切分、向量化并写入文档；同一 ID 重复写入会覆盖并清理多余旧块
func (e *Engine) Index(ctx context.Context, req IndexRequest) (*IndexResult, error) {
	ctx = orBackground(ctx)
	docs, err := prepareDocuments(req.Documents)
	if err != nil {
		return nil, err
	}
	splitter := req.Splitter
	if splitter == nil {
		splitter = e.defaultSplitter
	}
	// 先完成切分与边界校验，避免非法请求创建空集合
	prepared := make([]preparedDocument, 0, len(docs))
	texts := make([]string, 0)
	for _, doc := range docs {
		chunks, err := splitter.Split(doc.Content)
		if err != nil {
			return nil, err
		}
		// 自定义策略的输出同样要走统一校验，避免空块或超长块写入向量库
		chunks, err = normalizeChunks(chunks)
		if err != nil {
			return nil, err
		}
		if len(chunks) > maxChunksPerDoc {
			return nil, errors.Sentinel(ErrInvalidArgument)
		}
		prepared = append(prepared, preparedDocument{doc: doc, chunks: chunks})
		texts = append(texts, chunks...)
	}
	if err := e.ensureCollection(ctx); err != nil {
		return nil, err
	}
	// 写入前读取旧块数，写入后据此清理尾部旧块
	oldCounts, err := e.oldChunkCounts(ctx, docs)
	if err != nil {
		return nil, err
	}
	vectors, tokens, err := e.embed(ctx, texts)
	if err != nil {
		return nil, err
	}
	records := make([]vectordb.Record, 0, len(texts))
	offset := 0
	for _, p := range prepared {
		for i, content := range p.chunks {
			records = append(records, vectordb.Record{
				ID:       chunkID(p.doc.ID, i),
				Vector:   toFloat32(vectors[offset+i]),
				Metadata: chunkMetadata(p.doc, i, len(p.chunks), content),
			})
		}
		offset += len(p.chunks)
	}
	if err := e.store.Upsert(ctx, e.collection, records); err != nil {
		return nil, err
	}
	for _, p := range prepared {
		if err := e.deleteStaleChunks(ctx, p.doc.ID, oldCounts[p.doc.ID], len(p.chunks)); err != nil {
			return nil, err
		}
	}
	return &IndexResult{Documents: len(prepared), Chunks: len(texts), Tokens: tokens}, nil
}

// DeleteDocument 删除文档的所有块；集合不存在时返回 nil
func (e *Engine) DeleteDocument(ctx context.Context, docID string) error {
	ctx = orBackground(ctx)
	id := strings.TrimSpace(docID)
	if err := validateDocID(id); err != nil {
		return err
	}
	err := e.store.DeleteByFilter(ctx, e.collection, map[string]string{MetadataKeyDocumentID: id})
	if errors.Is(err, vectordb.ErrNotFound) {
		return nil
	}
	return err
}

// chunkMetadata 合并文档元数据与保留键
func chunkMetadata(doc Document, index, count int, content string) map[string]string {
	md := make(map[string]string, len(doc.Metadata)+5)
	for k, v := range doc.Metadata {
		md[k] = v
	}
	md[MetadataKeyDocumentID] = doc.ID
	md[MetadataKeyChunkIndex] = strconv.Itoa(index)
	md[MetadataKeyChunkCount] = strconv.Itoa(count)
	md[MetadataKeySource] = doc.Source
	md[MetadataKeyContent] = content
	return md
}

// oldChunkCounts 读取各文档第 0 块的块数；集合不存在时返回空表
func (e *Engine) oldChunkCounts(ctx context.Context, docs []Document) (map[string]int, error) {
	ids := make([]string, len(docs))
	for i := range docs {
		ids[i] = chunkID(docs[i].ID, 0)
	}
	out := make(map[string]int, len(ids))
	for start := 0; start < len(ids); start += idBatchSize {
		end := start + idBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		records, err := e.store.Get(ctx, e.collection, ids[start:end])
		if err != nil {
			if errors.Is(err, vectordb.ErrNotFound) {
				return map[string]int{}, nil
			}
			return nil, err
		}
		for i := range records {
			docID := strings.TrimSpace(records[i].Metadata[MetadataKeyDocumentID])
			if docID == "" {
				continue
			}
			count, err := strconv.Atoi(records[i].Metadata[MetadataKeyChunkCount])
			if err != nil || count <= 0 {
				continue
			}
			if count > maxChunksPerDoc {
				count = maxChunksPerDoc
			}
			out[docID] = count
		}
	}
	return out, nil
}

// deleteStaleChunks 删除序号超出新块数的旧块
func (e *Engine) deleteStaleChunks(ctx context.Context, docID string, oldCount, newCount int) error {
	if oldCount <= newCount || oldCount > maxChunksPerDoc {
		return nil
	}
	ids := make([]string, 0, oldCount-newCount)
	for i := newCount; i < oldCount; i++ {
		ids = append(ids, chunkID(docID, i))
	}
	for start := 0; start < len(ids); start += idBatchSize {
		end := start + idBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		if err := e.store.Delete(ctx, e.collection, ids[start:end]); err != nil {
			return err
		}
	}
	return nil
}
