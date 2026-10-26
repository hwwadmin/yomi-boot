// 门面接口、Engine 实现与集合生命周期。
//
// @author zwei

package rag

import (
	"context"
	"sync"

	"gitee.com/hq_project/yomi-boot/component/ai/chunking"
	"gitee.com/hq_project/yomi-boot/component/ai/embedding"
	"gitee.com/hq_project/yomi-boot/component/ai/generation"
	"gitee.com/hq_project/yomi-boot/component/ai/rerank"
	"gitee.com/hq_project/yomi-boot/component/ai/retrieval"
	"gitee.com/hq_project/yomi-boot/component/ai/vectordb"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// Engine RAG 两条流程的门面接口；实现须并发安全
type Engine interface {
	// Index 路径①文档入库：切块 → 向量化 → 写入指定集合；集合不存在时自动创建
	//
	// 同一 ID 重复写入会覆盖同序号块，并清理本次新块集合之外的旧块。
	Index(ctx context.Context, collection string, req IndexRequest) (*IndexResult, error)

	// Retrieve 路径②文档检索增强：检索 → 域过滤 → 精重排 → 提示词拼接
	//
	// 集合不存在或为空时返回空 Chunks 与由 Generator 拼接的 Prompt。
	Retrieve(ctx context.Context, collection string, req RetrieveRequest) (*RetrieveResult, error)

	// DeleteDocument 删除指定集合中文档的所有块；集合不存在时返回 nil
	DeleteDocument(ctx context.Context, collection, docID string) error
	// EnsureCollection 确保集合存在；已存在时返回 nil
	EnsureCollection(ctx context.Context, collection string) error
	// DropCollection 删除集合；不存在时返回 nil
	DropCollection(ctx context.Context, collection string) error
}

// engine 是 Engine 的实现；构造后不可变，可并发调用
type engine struct {
	chunker    chunking.Chunker
	embedder   embedding.Client
	retriever  retrieval.Retriever
	filter     retrieval.HitFilter
	reranker   rerank.Reranker
	generator  generation.Generator
	store      vectordb.Store
	indexLocks [indexLockStripes]sync.Mutex
}

var _ Engine = (*engine)(nil)

// New 校验配置并构造 Engine；不访问网络、不创建集合
func New(cfg Config) (Engine, error) {
	return newEngine(cfg)
}

// newEngine 校验配置并装配 engine 实现
func newEngine(cfg Config) (*engine, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	filter, err := newChunkFilter(cfg.Store)
	if err != nil {
		return nil, err
	}
	return &engine{
		chunker:   cfg.Chunker,
		embedder:  cfg.Embedder,
		retriever: cfg.Retriever,
		filter:    filter,
		reranker:  cfg.Reranker,
		generator: cfg.Generator,
		store:     cfg.Store,
	}, nil
}

// EnsureCollection 实现 Engine
func (e *engine) EnsureCollection(ctx context.Context, collection string) error {
	name, err := normalizeCollection(collection)
	if err != nil {
		return err
	}
	return e.ensureCollection(orBackground(ctx), name)
}

// DropCollection 实现 Engine
func (e *engine) DropCollection(ctx context.Context, collection string) error {
	name, err := normalizeCollection(collection)
	if err != nil {
		return err
	}
	return e.store.DropCollection(orBackground(ctx), name)
}

// ensureCollection 查询并按需创建集合；并发创建撞车时视为成功
func (e *engine) ensureCollection(ctx context.Context, collection string) error {
	has, err := e.store.HasCollection(ctx, collection)
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	if err := e.store.CreateCollection(ctx, collection); err != nil {
		if errors.Is(err, vectordb.ErrAlreadyExists) {
			return nil
		}
		return err
	}
	return nil
}

// orBackground 返回可用的 ctx；nil 回退 Background
func orBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
