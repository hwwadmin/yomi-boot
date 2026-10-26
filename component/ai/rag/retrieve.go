// 路径②文档检索增强：检索 → 域过滤 → 精重排 → 提示词拼接。
//
// @author zwei

package rag

import (
	"context"
	"strconv"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/ai/generation"
	"gitee.com/hq_project/yomi-boot/component/ai/rerank"
	"gitee.com/hq_project/yomi-boot/component/ai/vectordb"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/logger"
)

// Retrieve 执行文档检索增强：检索 → 域过滤 → 精重排 → 提示词拼接；集合不存在或为空时返回空 Chunks 与 nil
//
// 检索段（入参校验、查询向量化、策略路由、阈值过滤）由 ai/retrieval 门面执行，
// 检索入参非法错误透传其哨兵；本方法在其上叠加文档域过滤：缺少块文本的命中忽略、
// 更新窗口内的旧块丢弃。随后把过滤后的块交由 Reranker 按 RerankTopN 精重排
// （无命中时跳过，重排契约要求至少 1 条候选），最后由 Generator 拼接为输入提示词。
func (e *Engine) Retrieve(ctx context.Context, collection string, req RetrieveRequest) (*RetrieveResult, error) {
	name, err := normalizeCollection(collection)
	if err != nil {
		return nil, err
	}
	if req.RerankTopN < 0 {
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
	ctx = orBackground(ctx)
	hits, err := e.retriever.Retrieve(ctx, name, req.RetrieveRequest)
	if err != nil {
		return nil, err
	}
	candidates := make([]vectordb.Hit, 0, len(hits))
	for _, hit := range hits {
		if strings.TrimSpace(hit.Metadata[MetadataKeyContent]) == "" {
			continue
		}
		candidates = append(candidates, hit)
	}
	// 读取命中文档当前版本与块总数，过滤更新窗口内尚未删除的旧块
	states, err := e.currentChunkStates(ctx, name, candidates)
	if err != nil {
		if errors.Is(err, vectordb.ErrNotFound) {
			return e.retrieveEmpty(ctx, req)
		}
		return nil, err
	}
	chunks := make([]Chunk, 0, len(candidates))
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
		chunks = append(chunks, hitToChunk(hit, strings.TrimSpace(hit.Metadata[MetadataKeyContent])))
	}
	chunks, err = e.rerankChunks(ctx, req.Query, chunks, req.RerankTopN)
	if err != nil {
		return nil, err
	}
	prompt, err := e.buildPrompt(ctx, req.Query, chunks)
	if err != nil {
		return nil, err
	}
	return &RetrieveResult{Prompt: prompt, Chunks: chunks}, nil
}

// retrieveEmpty 无命中时的收尾：跳过重排（重排契约要求至少 1 条候选），
// 仍以空上下文拼接，保证 Prompt 契约稳定
func (e *Engine) retrieveEmpty(ctx context.Context, req RetrieveRequest) (*RetrieveResult, error) {
	prompt, err := e.buildPrompt(ctx, req.Query, []Chunk{})
	if err != nil {
		return nil, err
	}
	return &RetrieveResult{Prompt: prompt, Chunks: []Chunk{}}, nil
}

// rerankChunks 精重排并按结果回填块；topN 为 0 表示全部保留
func (e *Engine) rerankChunks(ctx context.Context, query string, chunks []Chunk, topN int) ([]Chunk, error) {
	if len(chunks) == 0 {
		return chunks, nil
	}
	docs := make([]string, len(chunks))
	for i, c := range chunks {
		docs[i] = c.Content
	}
	resp, err := e.reranker.Rerank(ctx, rerank.RerankRequest{
		Query: strings.TrimSpace(query),
		Docs:  docs,
		TopN:  topN,
	})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, wrapSentinel(ErrBadResponse, errors.New("ai/rag 重排实现未返回结果"))
	}
	out := make([]Chunk, 0, len(resp.Results))
	for _, r := range resp.Results {
		if r.Index < 0 || r.Index >= len(chunks) {
			return nil, wrapSentinel(ErrBadResponse, errors.New("ai/rag 重排结果下标越界"))
		}
		c := chunks[r.Index]
		c.RerankScore = r.Score
		out = append(out, c)
	}
	return out, nil
}

// buildPrompt 把重排后的块交由 Generator 拼接为输入提示词
func (e *Engine) buildPrompt(ctx context.Context, query string, chunks []Chunk) (string, error) {
	contexts := make([]generation.Context, len(chunks))
	for i, c := range chunks {
		contexts[i] = generation.Context{Content: c.Content, Source: c.Source}
	}
	resp, err := e.generator.Generate(ctx, generation.GenerateRequest{
		Query:    strings.TrimSpace(query),
		Contexts: contexts,
	})
	if err != nil {
		return "", err
	}
	if resp == nil || strings.TrimSpace(resp.Prompt) == "" {
		return "", wrapSentinel(ErrBadResponse, errors.New("ai/rag 提示词拼接实现返回空提示词"))
	}
	return resp.Prompt, nil
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
