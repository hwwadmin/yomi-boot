// 路径②文档检索增强：检索（含域过滤）→ 精重排 → 提示词拼接。
//
// @author zwei

package rag

import (
	"context"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/ai/generation"
	"gitee.com/hq_project/yomi-boot/component/ai/rerank"
	"gitee.com/hq_project/yomi-boot/component/ai/vectordb"
)

// Retrieve 实现 Engine：检索（含域过滤）→ 精重排 → 提示词拼接
//
// 检索段由 ai/retrieval 门面执行（入参校验、查询向量化、策略路由、前置与阈值过滤、
// 后置域过滤），本包经 RetrieveRequest.PostFilter 注入文档域过滤器；检索错误透传其哨兵。
func (e *engine) Retrieve(ctx context.Context, collection string, req RetrieveRequest) (*RetrieveResult, error) {
	name, err := normalizeCollection(collection)
	if err != nil {
		return nil, err
	}
	if err := validateRerankTopN(req.RerankTopN); err != nil {
		return nil, err
	}
	ctx = orBackground(ctx)
	// 域过滤器交给检索层执行
	req.RetrieveRequest.PostFilter = e.filter
	hits, err := e.retriever.Retrieve(ctx, name, req.RetrieveRequest)
	if err != nil {
		return nil, err
	}
	chunks := decodeHits(hits)
	chunks, err = applyRerank(ctx, e.reranker, req.Query, chunks, req.RerankTopN)
	if err != nil {
		return nil, err
	}
	prompt, err := assemblePrompt(ctx, e.generator, req.Query, chunks)
	if err != nil {
		return nil, err
	}
	return &RetrieveResult{Prompt: prompt, Chunks: chunks}, nil
}

// decodeHits 将检索命中转为块
func decodeHits(hits []vectordb.Hit) []Chunk {
	chunks := make([]Chunk, 0, len(hits))
	for _, hit := range hits {
		chunks = append(chunks, decodeHit(hit))
	}
	return chunks
}

// applyRerank 精重排：候选块文本交 Reranker，结果按下标回取并回填 RerankScore；
// 无命中时跳过
func applyRerank(ctx context.Context, reranker rerank.Reranker, query string, chunks []Chunk, topN int) ([]Chunk, error) {
	if len(chunks) == 0 {
		return chunks, nil
	}
	docs := make([]string, len(chunks))
	for i, c := range chunks {
		docs[i] = c.Content
	}
	resp, err := reranker.Rerank(ctx, rerank.RerankRequest{
		Query: strings.TrimSpace(query),
		Docs:  docs,
		TopN:  topN,
	})
	if err != nil {
		return nil, err
	}
	scored, err := rerank.Apply(chunks, resp)
	if err != nil {
		return nil, err
	}
	out := make([]Chunk, len(scored))
	for i, s := range scored {
		s.Item.RerankScore = s.Score
		out[i] = s.Item
	}
	return out, nil
}

// assemblePrompt 提示词拼接：重排后的块交 Generator，并校验输出契约
func assemblePrompt(ctx context.Context, generator generation.Generator, query string, chunks []Chunk) (string, error) {
	contexts := make([]generation.Context, len(chunks))
	for i, c := range chunks {
		contexts[i] = generation.Context{Content: c.Content, Source: c.Source}
	}
	resp, err := generator.Generate(ctx, generation.GenerateRequest{
		Query:    strings.TrimSpace(query),
		Contexts: contexts,
	})
	if err != nil {
		return "", err
	}
	if err := generation.ValidateResponse(resp); err != nil {
		return "", err
	}
	return resp.Prompt, nil
}
