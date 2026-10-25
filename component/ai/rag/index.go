// 文档入库与删除。
//
// @author zwei

package rag

import (
	"context"
	"strconv"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/ai/chunking"
	"gitee.com/hq_project/yomi-boot/component/ai/vectordb"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// preparedDocument 切分后的文档
type preparedDocument struct {
	doc     Document
	chunks  []string
	version string // version 是本次写入的文档版本
}

// idBatchSize 是按 ID 读写时的单批上限，避免一次请求携带过多主键
const idBatchSize = 500

// indexGroupChunks 是单组写入的块数预算；控制向量与记录的峰值内存
const indexGroupChunks = 256

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
	chunker := req.Chunker
	if chunker == nil {
		chunker = e.defaultChunker
	}
	// 先完成切分与边界校验，避免非法请求创建空集合
	prepared := make([]preparedDocument, 0, len(docs))
	for _, doc := range docs {
		chunks, err := chunker.Chunk(doc.Content)
		if err != nil {
			return nil, err
		}
		// 内置策略已保证块非空且不超长；这里兜底自定义 Chunker，
		// 契约要求 Engine 返回 ErrInvalidArgument（errors.Is 亦可匹配 chunking.ErrInvalidArgument）
		chunks, err = chunking.NormalizeChunks(chunks)
		if err != nil {
			return nil, wrapSentinel(ErrInvalidArgument, err)
		}
		if len(chunks) > maxChunksPerDoc {
			return nil, errors.Sentinel(ErrInvalidArgument)
		}
		version, err := newVersion()
		if err != nil {
			return nil, wrapSentinel(ErrBadResponse, err)
		}
		prepared = append(prepared, preparedDocument{doc: doc, chunks: chunks, version: version})
	}
	result := &IndexResult{}
	// 按块预算分组写入，避免一次请求把全部向量与记录同时放在内存
	for _, group := range groupPrepared(prepared, indexGroupChunks) {
		tokens, err := e.indexGroup(ctx, name, group)
		if err != nil {
			return nil, err
		}
		result.Documents += len(group.docs)
		result.Chunks += len(group.texts)
		result.Tokens += tokens
	}
	return result, nil
}

// docGroup 是一次写入的文档批次；texts 是组内全部块文本
type docGroup struct {
	docs  []preparedDocument
	texts []string
}

// groupPrepared 按块数预算把文档分组；单文档超过预算时独占一组
func groupPrepared(prepared []preparedDocument, maxChunks int) []docGroup {
	groups := make([]docGroup, 0, len(prepared))
	current := docGroup{}
	for _, p := range prepared {
		if len(current.texts) > 0 && len(current.texts)+len(p.chunks) > maxChunks {
			groups = append(groups, current)
			current = docGroup{}
		}
		current.docs = append(current.docs, p)
		current.texts = append(current.texts, p.chunks...)
	}
	if len(current.docs) > 0 {
		groups = append(groups, current)
	}
	return groups
}

// indexGroup 对一组文档执行向量化、写入与旧块清理；返回本组向量化用量
func (e *Engine) indexGroup(ctx context.Context, collection string, group docGroup) (int, error) {
	vectors, tokens, err := e.embed(ctx, group.texts)
	if err != nil {
		return 0, err
	}
	records := make([]vectordb.Record, 0, len(group.texts))
	offset := 0
	for _, p := range group.docs {
		for i, content := range p.chunks {
			records = append(records, vectordb.Record{
				ID:       chunkID(p.doc.ID, i),
				Vector:   toFloat32(vectors[offset+i]),
				Metadata: chunkMetadata(p.doc, i, len(p.chunks), p.version, content),
			})
		}
		offset += len(p.chunks)
	}
	// 写入阶段按文档加锁；条带锁只保证进程内，跨实例需调用方串行
	unlock := e.lockDocuments(collection, group.docs)
	defer unlock()
	if err := e.ensureCollection(ctx, collection); err != nil {
		return 0, err
	}
	// 先写新块：Upsert 失败时不会删除旧块，可能出现新旧块并存，重试可自愈
	if err := e.store.Upsert(ctx, collection, records); err != nil {
		return 0, err
	}
	// 写入成功后列出各文档现有块，删除不在本次新块集合中的旧块；
	// 删除失败或进程崩溃时旧块仍在，下次 Index 会重新枚举并清理，可自愈
	for _, p := range group.docs {
		if err := e.deleteStaleChunks(ctx, collection, p.doc.ID, len(p.chunks)); err != nil {
			return 0, err
		}
	}
	return tokens, nil
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

// chunkMetadata 合并文档元数据与保留键；total 是块总数，version 是本次写入版本
func chunkMetadata(doc Document, index, total int, version, content string) map[string]string {
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
