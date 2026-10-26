// 路径①文档入库：切块 → 向量化 → 写入 → 旧块清理。
//
// @author zwei

package rag

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sort"

	"gitee.com/hq_project/yomi-boot/component/ai/chunking"
	"gitee.com/hq_project/yomi-boot/component/ai/embedding"
	"gitee.com/hq_project/yomi-boot/component/ai/vectordb"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

const (
	// indexBatchChunks 是单批写入的块数预算；控制向量与记录的峰值内存
	indexBatchChunks = 256
	// indexLockStripes 是 Index 写入阶段的锁条数；同一文档命中同一条带
	indexLockStripes = 64
)

// newVersion 生成文档写入版本；检索侧据此区分新旧块
func newVersion() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// chunkedDocument 切块后的文档
type chunkedDocument struct {
	doc     Document
	chunks  []string
	version string // version 是本次写入的文档版本
}

// indexBatch 是一次写入的文档批次；texts 是批内全部块文本
type indexBatch struct {
	docs  []chunkedDocument
	texts []string
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
	// 先切块并校验，全部通过后再写入
	prepared, err := chunkDocuments(chunker, docs)
	if err != nil {
		return nil, err
	}
	result := &IndexResult{}
	for _, batch := range buildIndexBatches(prepared, indexBatchChunks) {
		tokens, err := e.writeBatch(ctx, name, batch)
		if err != nil {
			return nil, err
		}
		result.Documents += len(batch.docs)
		result.Chunks += len(batch.texts)
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
		version, err := newVersion()
		if err != nil {
			return nil, wrapSentinel(ErrBadResponse, err)
		}
		prepared = append(prepared, chunkedDocument{doc: doc, chunks: chunks, version: version})
	}
	return prepared, nil
}

// buildIndexBatches 按块数预算把文档分组；单文档超过预算时独占一组
func buildIndexBatches(prepared []chunkedDocument, maxChunks int) []indexBatch {
	batches := make([]indexBatch, 0, len(prepared))
	current := indexBatch{}
	for _, p := range prepared {
		if len(current.texts) > 0 && len(current.texts)+len(p.chunks) > maxChunks {
			batches = append(batches, current)
			current = indexBatch{}
		}
		current.docs = append(current.docs, p)
		current.texts = append(current.texts, p.chunks...)
	}
	if len(current.docs) > 0 {
		batches = append(batches, current)
	}
	return batches
}

// writeBatch 对一批文档执行向量化、写入与旧块清理；返回本批向量化用量
func (e *engine) writeBatch(ctx context.Context, collection string, batch indexBatch) (int, error) {
	resp, err := e.embedder.Embed(ctx, embedding.Request{Inputs: batch.texts})
	if err != nil {
		return 0, err
	}
	if resp == nil || len(resp.Vectors) != len(batch.texts) {
		return 0, wrapSentinel(ErrBadResponse, errors.New("ai/rag 向量化返回条数与输入不一致"))
	}
	records := make([]vectordb.Record, 0, len(batch.texts))
	offset := 0
	for _, p := range batch.docs {
		for i, content := range p.chunks {
			records = append(records, vectordb.Record{
				ID:     chunkID(p.doc.ID, i),
				Vector: toFloat32(resp.Vectors[offset+i]),
				// 块原文写入 Text，供混合检索的 BM25 一路命中
				Text:     content,
				Metadata: encodeChunkMetadata(p.doc, i, len(p.chunks), p.version, content),
			})
		}
		offset += len(p.chunks)
	}
	// 同一文档的写入按条带锁串行（进程内）
	unlock := e.lockDocuments(collection, batch.docs)
	defer unlock()
	if err := e.ensureCollection(ctx, collection); err != nil {
		return 0, err
	}
	// 先写新块再清理旧块：写入失败时旧块保持完整，重试即可
	if err := e.store.Upsert(ctx, collection, records); err != nil {
		return 0, err
	}
	// 写入成功后删除该文档多余的旧块；清理失败时下次 Index 会重新清理
	for _, p := range batch.docs {
		if err := e.deleteStaleChunks(ctx, collection, p.doc.ID, len(p.chunks)); err != nil {
			return 0, err
		}
	}
	tokens := 0
	if resp.Usage != nil {
		tokens = resp.Usage.TotalTokens
	}
	return tokens, nil
}

// deleteStaleChunks 删除指定集合中不属于本次新块集合的旧块
func (e *engine) deleteStaleChunks(ctx context.Context, collection, docID string, newCount int) error {
	keep := make(map[string]struct{}, newCount)
	for i := 0; i < newCount; i++ {
		keep[chunkID(docID, i)] = struct{}{}
	}
	ids, err := e.store.ListIDs(ctx, collection, map[string]string{MetadataKeyDocumentID: docID})
	if err != nil {
		// 列出失败时直接上报
		return err
	}
	stale := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := keep[id]; ok {
			continue
		}
		stale = append(stale, id)
	}
	if len(stale) == 0 {
		return nil
	}
	return e.store.Delete(ctx, collection, stale)
}

// lockDocuments 按文档获取写入锁；条带按序加锁避免死锁，返回释放函数
func (e *engine) lockDocuments(collection string, docs []chunkedDocument) func() {
	stripes := make([]int, 0, len(docs))
	seen := make(map[int]struct{}, len(docs))
	for _, doc := range docs {
		stripe := int(indexLockHash(collection, doc.doc.ID) % indexLockStripes)
		if _, ok := seen[stripe]; ok {
			continue
		}
		seen[stripe] = struct{}{}
		stripes = append(stripes, stripe)
	}
	sort.Ints(stripes)
	for _, stripe := range stripes {
		e.indexLocks[stripe].Lock()
	}
	return func() {
		for i := len(stripes) - 1; i >= 0; i-- {
			e.indexLocks[stripes[i]].Unlock()
		}
	}
}

// indexLockHash 计算集合与文档 ID 的 FNV-1a 哈希
func indexLockHash(collection, docID string) uint32 {
	const (
		offset32 = 2166136261
		prime32  = 16777619
	)
	h := uint32(offset32)
	for i := 0; i < len(collection); i++ {
		h ^= uint32(collection[i])
		h *= prime32
	}
	h *= prime32
	for i := 0; i < len(docID); i++ {
		h ^= uint32(docID[i])
		h *= prime32
	}
	return h
}

// toFloat32 将向量化输出转为向量库所需的 float32
func toFloat32(vec []float64) []float32 {
	out := make([]float32, len(vec))
	for i, v := range vec {
		out[i] = float32(v)
	}
	return out
}
