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

// Index 切分、向量化并写入指定集合；集合不存在时自动创建
//
// 同一 ID 重复写入会覆盖同序号块，并清理本次新块集合之外的旧块。
func (e *Engine) Index(ctx context.Context, collection string, req IndexRequest) (*IndexResult, error) {
	ctx = orBackground(ctx)
	name, err := normalizeCollection(collection)
	if err != nil {
		return nil, err
	}
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
				Metadata: chunkMetadata(p.doc, i, content),
			})
		}
		offset += len(p.chunks)
	}
	// 写入阶段按文档加锁；条带锁只保证进程内，跨实例需调用方串行
	unlock := e.lockDocuments(name, prepared)
	defer unlock()
	if err := e.ensureCollection(ctx, name); err != nil {
		return nil, err
	}
	// 先写新块：Upsert 失败时旧块保持完整，不会出现“旧块已删、新块没写进去”
	if err := e.store.Upsert(ctx, name, records); err != nil {
		return nil, err
	}
	// 写入成功后列出该文档现有块，删除不在本次新块集合中的旧块；
	// 删除失败或进程崩溃时旧块仍在，下次 Index 会重新枚举并清理，可自愈
	for _, p := range prepared {
		if err := e.deleteStaleChunks(ctx, name, p.doc.ID, len(p.chunks)); err != nil {
			return nil, err
		}
	}
	return &IndexResult{Documents: len(prepared), Chunks: len(texts), Tokens: tokens}, nil
}

// DeleteDocument 删除指定集合中文档的所有块；集合不存在时返回 nil
func (e *Engine) DeleteDocument(ctx context.Context, collection, docID string) error {
	ctx = orBackground(ctx)
	name, err := normalizeCollection(collection)
	if err != nil {
		return err
	}
	id := strings.TrimSpace(docID)
	if err := validateDocID(id); err != nil {
		return err
	}
	err = e.store.DeleteByFilter(ctx, name, map[string]string{MetadataKeyDocumentID: id})
	if errors.Is(err, vectordb.ErrNotFound) {
		return nil
	}
	return err
}

// chunkMetadata 合并文档元数据与保留键
func chunkMetadata(doc Document, index int, content string) map[string]string {
	md := make(map[string]string, len(doc.Metadata)+4)
	for k, v := range doc.Metadata {
		md[k] = v
	}
	md[MetadataKeyDocumentID] = doc.ID
	md[MetadataKeyChunkIndex] = strconv.Itoa(index)
	md[MetadataKeySource] = doc.Source
	md[MetadataKeyContent] = content
	return md
}

// deleteStaleChunks 删除指定集合中不属于本次新块集合的旧块
func (e *Engine) deleteStaleChunks(ctx context.Context, collection, docID string, newCount int) error {
	keep := make(map[string]struct{}, newCount)
	for i := 0; i < newCount; i++ {
		keep[chunkID(docID, i)] = struct{}{}
	}
	ids, err := e.store.ListIDs(ctx, collection, map[string]string{MetadataKeyDocumentID: docID})
	if err != nil {
		// 写入已成功，集合此时不应消失；列出失败必须上报，不能把失败当成功
		return err
	}
	stale := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := keep[id]; ok {
			continue
		}
		stale = append(stale, id)
	}
	for start := 0; start < len(stale); start += idBatchSize {
		end := start + idBatchSize
		if end > len(stale) {
			end = len(stale)
		}
		if err := e.store.Delete(ctx, collection, stale[start:end]); err != nil {
			return err
		}
	}
	return nil
}
