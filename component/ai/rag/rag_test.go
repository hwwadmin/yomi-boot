// Engine 主流程测试。
//
// @author zwei

package rag

import (
	"context"
	"math"
	"strings"
	"sync"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/ai/chunking"
	"gitee.com/hq_project/yomi-boot/component/ai/embedding"
	"gitee.com/hq_project/yomi-boot/component/ai/generation"
	"gitee.com/hq_project/yomi-boot/component/ai/rerank"
	"gitee.com/hq_project/yomi-boot/component/ai/retrieval"
	"gitee.com/hq_project/yomi-boot/component/ai/vectordb"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

const (
	testCollection = "rag_test"
	testDimension  = 3
)

// fakeEmbedder 按关键词返回确定性向量，便于断言检索排序
type fakeEmbedder struct {
	mu    sync.Mutex
	dim   int
	err   error
	calls int
}

// Embed 实现 embedding.Client
func (f *fakeEmbedder) Embed(ctx context.Context, req embedding.Request) (*embedding.Response, error) {
	f.mu.Lock()
	f.calls++
	err := f.err
	dim := f.dim
	f.mu.Unlock()
	if err != nil {
		return nil, err
	}
	vectors := make([][]float64, len(req.Inputs))
	for i, input := range req.Inputs {
		vectors[i] = fakeVector(input, dim)
	}
	return &embedding.Response{
		Model:   "fake-embed",
		Vectors: vectors,
		Usage:   &embedding.TokenUsage{PromptTokens: len(req.Inputs), TotalTokens: len(req.Inputs)},
	}, nil
}

// setDim 修改后续向量维度，用于验证维度锁定
func (f *fakeEmbedder) setDim(dim int) {
	f.mu.Lock()
	f.dim = dim
	f.mu.Unlock()
}

// callCount 返回调用次数
func (f *fakeEmbedder) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// fakeVector 将关键词映射到不同维度；其余文本落在最后一维
func fakeVector(text string, dim int) []float64 {
	v := make([]float64, dim)
	if dim == 0 {
		return v
	}
	switch {
	case strings.Contains(text, "苹果"):
		v[0] = 1
	case strings.Contains(text, "香蕉") && dim > 1:
		v[1] = 1
	default:
		v[dim-1] = 1
	}
	return v
}

// fakeReranker 测试替身：按打分函数重排，复用 rerank.RankScores 整理结果
type fakeReranker struct {
	score func(i int, doc string) float32 // score 按候选下标打分；nil 恒 0（等价保序）
	err   error                           // err 非空时直接返回错误
	resp  *rerank.RerankResponse          // resp 非空时原样返回，用于模拟实现违约
	calls int
}

// Rerank 实现 rerank.Reranker
func (f *fakeReranker) Rerank(ctx context.Context, req rerank.RerankRequest) (*rerank.RerankResponse, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if f.resp != nil {
		return f.resp, nil
	}
	scores := make([]float32, len(req.Docs))
	for i, doc := range req.Docs {
		if f.score != nil {
			scores[i] = f.score(i, doc)
		}
	}
	results, err := rerank.RankScores(scores, req.TopN)
	if err != nil {
		return nil, err
	}
	return &rerank.RerankResponse{Results: results}, nil
}

// fakeGenerator 测试替身：记录最近一次请求并返回固定提示词
type fakeGenerator struct {
	req    generation.GenerateRequest                  // req 是最近一次收到的请求
	err    error                                       // err 非空时直接返回错误
	prompt func(req generation.GenerateRequest) string // prompt 非空时以其结果作为输出
	calls  int
}

// Generate 实现 generation.Generator
func (f *fakeGenerator) Generate(ctx context.Context, req generation.GenerateRequest) (*generation.GenerateResponse, error) {
	f.calls++
	f.req = req
	if f.err != nil {
		return nil, f.err
	}
	if f.prompt != nil {
		return &generation.GenerateResponse{Prompt: f.prompt(req)}, nil
	}
	return &generation.GenerateResponse{Prompt: "测试提示词"}, nil
}

// getErrStore 让 Get 返回不可用，用于验证检索辅助查询的降级
type getErrStore struct{ vectordb.Store }

// Get 模拟辅助版本/计数查询失败
func (s getErrStore) Get(ctx context.Context, collection string, ids []string) ([]vectordb.Record, error) {
	return nil, errors.Sentinel(vectordb.ErrUnavailable)
}

// ctxErrStore 让 Get 返回 context.Canceled，用于验证取消不降级
type ctxErrStore struct{ vectordb.Store }

// Get 模拟辅助查询遇到上下文取消
func (s ctxErrStore) Get(ctx context.Context, collection string, ids []string) ([]vectordb.Record, error) {
	return nil, context.Canceled
}

// stubChunker 返回固定块，用于验证自定义策略注入
type stubChunker struct {
	chunks []string
	err    error
	calls  int
}

// Chunk 实现 Chunker
func (s *stubChunker) Chunk(text string) ([]string, error) {
	s.calls++
	return s.chunks, s.err
}

// newTestStore 创建指定维度的内存向量库
func newTestStore(t *testing.T, dim int) vectordb.Store {
	t.Helper()
	store, err := vectordb.New(vectordb.Config{Provider: vectordb.ProviderMemory, Dimension: dim})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// mustTestChunker 构造默认测试切块器
func mustTestChunker(t *testing.T) chunking.Chunker {
	t.Helper()
	c, err := chunking.New(chunking.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// newTestRetriever 构造基于指定依赖的通用检索门面
func newTestRetriever(t *testing.T, emb embedding.Client, store vectordb.Store) retrieval.Retriever {
	t.Helper()
	r, err := retrieval.New(retrieval.Config{Embedder: emb, Store: store})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// newTestEngineWithConfig 校验并构造引擎实现，失败即终止测试
func newTestEngineWithConfig(t *testing.T, cfg Config) *engine {
	t.Helper()
	e, err := newEngine(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// newTestEngineWithDeps 用显式 Embedder/Store 构造全配置引擎（占位重排与拼接）
func newTestEngineWithDeps(t *testing.T, emb embedding.Client, store vectordb.Store) *engine {
	t.Helper()
	return newTestEngineWithConfig(t, Config{
		Chunker:   mustTestChunker(t),
		Embedder:  emb,
		Retriever: newTestRetriever(t, emb, store),
		Reranker:  rerank.NewPassThrough(),
		Generator: generation.NewBasic(),
		Store:     store,
	})
}

// newTestEngine 创建默认测试引擎
func newTestEngine(t *testing.T) (*engine, *fakeEmbedder, vectordb.Store) {
	t.Helper()
	store := newTestStore(t, testDimension)
	emb := &fakeEmbedder{dim: testDimension}
	return newTestEngineWithDeps(t, emb, store), emb, store
}

// TestIndexAndRetrieve 覆盖入库、排序、过滤、阈值与返回字段
func TestIndexAndRetrieve(t *testing.T) {
	t.Parallel()
	e, emb, _ := newTestEngine(t)
	ctx := context.Background()
	res, err := e.Index(ctx, testCollection, IndexRequest{Documents: []Document{
		{
			ID:       "doc-apple",
			Content:  "苹果是一种水果。苹果富含维生素。",
			Source:   "fruit/apple.md",
			Metadata: map[string]string{"lang": "zh"},
		},
		{ID: "doc-banana", Content: "香蕉是黄色的。香蕉生长在热带。", Source: "fruit/banana.md"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Documents != 2 || res.Chunks != 2 || res.Tokens != 2 {
		t.Fatalf("入库结果: %+v", res)
	}
	has, err := e.store.HasCollection(ctx, testCollection)
	if err != nil || !has {
		t.Fatalf("集合未创建: has=%v err=%v", has, err)
	}
	out, err := e.Retrieve(ctx, testCollection, RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{Query: "苹果", TopK: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	chunks := out.Chunks
	if len(chunks) != 2 {
		t.Fatalf("检索结果数 = %d", len(chunks))
	}
	top := chunks[0]
	if top.DocumentID != "doc-apple" || top.ID != "doc-apple#0" || top.Index != 0 {
		t.Fatalf("命中字段: %+v", top)
	}
	if top.Content != "苹果是一种水果。苹果富含维生素。" || top.Source != "fruit/apple.md" {
		t.Fatalf("命中内容: %+v", top)
	}
	if top.Metadata["lang"] != "zh" {
		t.Fatalf("自定义元数据: %+v", top.Metadata)
	}
	if _, ok := top.Metadata[MetadataKeyDocumentID]; ok {
		t.Fatalf("保留键未剥离: %+v", top.Metadata)
	}
	if top.Score <= chunks[1].Score {
		t.Fatalf("排序错误: %+v", chunks)
	}
	// 两篇文档各一次写入向量化，首次检索一次查询向量化
	if emb.callCount() != 3 {
		t.Fatalf("向量化调用 = %d，期望 3", emb.callCount())
	}

	out, err = e.Retrieve(ctx, testCollection, RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{
			Query:  "水果",
			TopK:   10,
			Filter: map[string]string{MetadataKeySource: "fruit/banana.md"},
		},
	})
	if err != nil || len(out.Chunks) != 1 || out.Chunks[0].DocumentID != "doc-banana" {
		t.Fatalf("来源过滤: %+v err=%v", out, err)
	}
	out, err = e.Retrieve(ctx, testCollection, RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{
			Query:  "水果",
			TopK:   10,
			Filter: map[string]string{"lang": "zh"},
		},
	})
	if err != nil || len(out.Chunks) != 1 || out.Chunks[0].DocumentID != "doc-apple" {
		t.Fatalf("自定义元数据过滤: %+v err=%v", out, err)
	}
	out, err = e.Retrieve(ctx, testCollection, RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{Query: "苹果", TopK: 10, MinScore: 0.02},
	})
	if err != nil || len(out.Chunks) != 1 || out.Chunks[0].DocumentID != "doc-apple" {
		t.Fatalf("阈值过滤: %+v err=%v", out, err)
	}
	out, err = e.Retrieve(ctx, testCollection, RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{Query: "苹果", TopK: 10},
	})
	if err != nil || len(out.Chunks) != 2 {
		t.Fatalf("不设阈值: %+v err=%v", out, err)
	}
	out, err = e.Retrieve(ctx, testCollection, RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{Query: "水果", TopK: 1},
	})
	if err != nil || len(out.Chunks) != 1 {
		t.Fatalf("TopK 生效: %+v err=%v", out, err)
	}
}

// TestRetrievePromptAndChunks 覆盖检索增强输出：Basic 拼接的 Prompt 与重排后 Chunks
func TestRetrievePromptAndChunks(t *testing.T) {
	t.Parallel()
	e, _, _ := newTestEngine(t)
	ctx := context.Background()
	if _, err := e.Index(ctx, testCollection, IndexRequest{Documents: []Document{
		{ID: "doc-apple", Content: "苹果是一种水果。", Source: "fruit/apple.md"},
	}}); err != nil {
		t.Fatal(err)
	}
	out, err := e.Retrieve(ctx, testCollection, RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{Query: "苹果", TopK: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Chunks) != 1 {
		t.Fatalf("检索结果数 = %d", len(out.Chunks))
	}
	want := "苹果\n\n参考资料：\n[1] 苹果是一种水果。（来源：fruit/apple.md）"
	if out.Prompt != want {
		t.Fatalf("Prompt = %q，期望 %q", out.Prompt, want)
	}
}

// TestMultipleCollections 覆盖同一 Engine 下多业务集合的数据隔离
func TestMultipleCollections(t *testing.T) {
	t.Parallel()
	e, _, store := newTestEngine(t)
	ctx := context.Background()
	if _, err := e.Index(ctx, "kb_sales", IndexRequest{Documents: []Document{
		{ID: "sales-1", Content: "苹果报价。"},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Index(ctx, "kb_hr", IndexRequest{Documents: []Document{
		{ID: "hr-1", Content: "香蕉福利。"},
	}}); err != nil {
		t.Fatal(err)
	}
	out, err := e.Retrieve(ctx, "kb_sales", RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{Query: "香蕉", TopK: 10, MinScore: 0.02},
	})
	if err != nil || len(out.Chunks) != 0 {
		t.Fatalf("销售库不应命中人事文档: %+v err=%v", out, err)
	}
	out, err = e.Retrieve(ctx, "kb_hr", RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{Query: "香蕉", TopK: 10, MinScore: 0.02},
	})
	if err != nil || len(out.Chunks) != 1 || out.Chunks[0].DocumentID != "hr-1" {
		t.Fatalf("人事库命中: %+v err=%v", out, err)
	}
	// 文档删除与集合删除不属于两条流程，直接走 Store
	if err := store.DeleteByFilter(ctx, "kb_sales", map[string]string{MetadataKeyDocumentID: "sales-1"}); err != nil {
		t.Fatal(err)
	}
	out, err = e.Retrieve(ctx, "kb_hr", RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{Query: "香蕉", TopK: 10, MinScore: 0.02},
	})
	if err != nil || len(out.Chunks) != 1 {
		t.Fatalf("删除销售库文档不应影响人事库: %+v err=%v", out, err)
	}
	if err := store.DropCollection(ctx, "kb_sales"); err != nil {
		t.Fatal(err)
	}
	hasSales, err := store.HasCollection(ctx, "kb_sales")
	if err != nil || hasSales {
		t.Fatalf("销售库未删除: has=%v err=%v", hasSales, err)
	}
	hasHR, err := store.HasCollection(ctx, "kb_hr")
	if err != nil || !hasHR {
		t.Fatalf("人事库应保留: has=%v err=%v", hasHR, err)
	}
}

// TestRetrieveEmptyCollection 覆盖集合不存在与空集合
func TestRetrieveEmptyCollection(t *testing.T) {
	t.Parallel()
	e, _, _ := newTestEngine(t)
	out, err := e.Retrieve(context.Background(), testCollection, RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{Query: "苹果", TopK: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Chunks == nil || len(out.Chunks) != 0 {
		t.Fatalf("空检索应返回非 nil 空切片: %#v", out.Chunks)
	}
	// 无命中时仍以空上下文拼接，Basic 占位下 Prompt 即问题原文
	if out.Prompt != "苹果" {
		t.Fatalf("空检索 Prompt = %q", out.Prompt)
	}
}

// TestIndexReplacesStaleChunks 覆盖重复入库时清理多余旧块
func TestIndexReplacesStaleChunks(t *testing.T) {
	t.Parallel()
	e, _, store := newTestEngine(t)
	chunker, err := chunking.New(chunking.Config{
		Recursive: chunking.RecursiveConfig{ChunkSize: 6, ChunkOverlap: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	res, err := e.Index(ctx, testCollection, IndexRequest{
		Documents: []Document{{ID: "doc", Content: "苹果。苹果。苹果。苹果。"}},
		Chunker:   chunker,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Chunks < 3 {
		t.Fatalf("首次切块 = %d，期望至少 3", res.Chunks)
	}
	oldCount := res.Chunks
	res, err = e.Index(ctx, testCollection, IndexRequest{
		Documents: []Document{{ID: "doc", Content: "苹果。"}},
		Chunker:   chunker,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Chunks != 1 {
		t.Fatalf("二次切块 = %d，期望 1", res.Chunks)
	}
	got, err := store.Get(ctx, testCollection, []string{chunkID("doc", 0), chunkID("doc", 1)})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != chunkID("doc", 0) {
		t.Fatalf("旧块未清理: %+v", got)
	}
	got, err = store.Get(ctx, testCollection, []string{chunkID("doc", oldCount-1)})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("尾部旧块仍存在: %+v", got)
	}
	out, err := e.Retrieve(ctx, testCollection, RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{Query: "苹果", TopK: 20},
	})
	if err != nil || len(out.Chunks) != 1 || out.Chunks[0].DocumentID != "doc" {
		t.Fatalf("重复入库后检索: %+v err=%v", out, err)
	}
}

// TestRetrieveFiltersStaleChunks 验证索引收缩窗口内不会返回当前块总数之外的旧块
func TestRetrieveFiltersStaleChunks(t *testing.T) {
	t.Parallel()
	e, _, store := newTestEngine(t)
	ctx := context.Background()
	doc := Document{ID: "doc", Content: "苹果"}
	old := make([]vectordb.Record, 8)
	for i := range old {
		old[i] = vectordb.Record{
			ID:       chunkID("doc", i),
			Vector:   []float32{1, 0, 0},
			Metadata: encodeChunkMetadata(doc, i, len(old), "old", "旧内容"),
		}
	}
	if err := store.CreateCollection(ctx, testCollection); err != nil {
		t.Fatal(err)
	}
	if err := store.Upsert(ctx, testCollection, old); err != nil {
		t.Fatal(err)
	}
	// 模拟 Upsert 已完成、旧块尚未删除的窗口：0..2 是新块，3..7 仍是旧块
	fresh := make([]vectordb.Record, 3)
	for i := range fresh {
		fresh[i] = vectordb.Record{
			ID:       chunkID("doc", i),
			Vector:   []float32{1, 0, 0},
			Metadata: encodeChunkMetadata(doc, i, len(fresh), "new", "新内容"),
		}
	}
	if err := store.Upsert(ctx, testCollection, fresh); err != nil {
		t.Fatal(err)
	}
	out, err := e.Retrieve(ctx, testCollection, RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{Query: "苹果", TopK: 20},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Chunks) != len(fresh) {
		t.Fatalf("chunks=%d %+v", len(out.Chunks), out.Chunks)
	}
	for _, chunk := range out.Chunks {
		if chunk.Index >= len(fresh) || chunk.Content != "新内容" {
			t.Fatalf("命中旧块: %+v", chunk)
		}
	}
}

// TestRetrieveFiltersOldVersionAfterGrowthPartialWrite 验证新版块数变多且只写入部分新块时，旧版本块仍被过滤
func TestRetrieveFiltersOldVersionAfterGrowthPartialWrite(t *testing.T) {
	t.Parallel()
	e, _, store := newTestEngine(t)
	ctx := context.Background()
	doc := Document{ID: "doc", Content: "苹果"}
	old := make([]vectordb.Record, 3)
	for i := range old {
		old[i] = vectordb.Record{
			ID:       chunkID("doc", i),
			Vector:   []float32{1, 0, 0},
			Metadata: encodeChunkMetadata(doc, i, len(old), "old", "旧内容"),
		}
	}
	if err := store.CreateCollection(ctx, testCollection); err != nil {
		t.Fatal(err)
	}
	if err := store.Upsert(ctx, testCollection, old); err != nil {
		t.Fatal(err)
	}
	// 新版共 8 块，但只写入 0..1；旧块 2 仍在
	fresh := make([]vectordb.Record, 2)
	for i := range fresh {
		fresh[i] = vectordb.Record{
			ID:       chunkID("doc", i),
			Vector:   []float32{1, 0, 0},
			Metadata: encodeChunkMetadata(doc, i, 8, "new", "新内容"),
		}
	}
	if err := store.Upsert(ctx, testCollection, fresh); err != nil {
		t.Fatal(err)
	}
	out, err := e.Retrieve(ctx, testCollection, RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{Query: "苹果", TopK: 20},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Chunks) != len(fresh) {
		t.Fatalf("chunks=%d %+v", len(out.Chunks), out.Chunks)
	}
	for _, chunk := range out.Chunks {
		if chunk.Index >= len(fresh) || chunk.Content != "新内容" {
			t.Fatalf("命中旧版本块: %+v", chunk)
		}
	}
}

// TestRetrieveDegradesWhenChunkStateFails 验证版本/计数查询失败时降级为不过滤而不是检索失败
func TestRetrieveDegradesWhenChunkStateFails(t *testing.T) {
	t.Parallel()
	store := newTestStore(t, testDimension)
	e := newTestEngineWithDeps(t, &fakeEmbedder{dim: testDimension}, getErrStore{Store: store})
	ctx := context.Background()
	doc := Document{ID: "doc", Content: "苹果"}
	if err := store.CreateCollection(ctx, testCollection); err != nil {
		t.Fatal(err)
	}
	if err := store.Upsert(ctx, testCollection, []vectordb.Record{{
		ID:       chunkID("doc", 0),
		Vector:   []float32{1, 0, 0},
		Metadata: encodeChunkMetadata(doc, 0, 1, "v1", "苹果"),
	}}); err != nil {
		t.Fatal(err)
	}
	out, err := e.Retrieve(ctx, testCollection, RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{Query: "苹果", TopK: 5},
	})
	if err != nil {
		t.Fatalf("版本/计数查询失败不应导致检索失败: %v", err)
	}
	if len(out.Chunks) != 1 || out.Chunks[0].DocumentID != "doc" {
		t.Fatalf("chunks=%+v", out.Chunks)
	}
}

// TestRetrieveReturnsContextErrorFromChunkStateQuery 验证辅助查询遇到取消时上抛而不是降级
func TestRetrieveReturnsContextErrorFromChunkStateQuery(t *testing.T) {
	t.Parallel()
	store := newTestStore(t, testDimension)
	e := newTestEngineWithDeps(t, &fakeEmbedder{dim: testDimension}, ctxErrStore{Store: store})
	ctx := context.Background()
	doc := Document{ID: "doc", Content: "苹果"}
	if err := store.CreateCollection(ctx, testCollection); err != nil {
		t.Fatal(err)
	}
	if err := store.Upsert(ctx, testCollection, []vectordb.Record{{
		ID:       chunkID("doc", 0),
		Vector:   []float32{1, 0, 0},
		Metadata: encodeChunkMetadata(doc, 0, 1, "v1", "苹果"),
	}}); err != nil {
		t.Fatal(err)
	}
	out, err := e.Retrieve(ctx, testCollection, RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{Query: "苹果", TopK: 5},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("期望 context.Canceled，实际 %v", err)
	}
	if out != nil {
		t.Fatalf("取消时不应返回结果: %+v", out)
	}
}

// TestDocumentDeletionViaStore 覆盖不经门面的文档删除：按保留键 rag_doc_id 经 Store 过滤删除
func TestDocumentDeletionViaStore(t *testing.T) {
	t.Parallel()
	e, _, store := newTestEngine(t)
	ctx := context.Background()
	if _, err := e.Index(ctx, testCollection, IndexRequest{Documents: []Document{
		{ID: "doc", Content: "苹果是一种水果。"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteByFilter(ctx, testCollection, map[string]string{MetadataKeyDocumentID: "doc"}); err != nil {
		t.Fatal(err)
	}
	out, err := e.Retrieve(ctx, testCollection, RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{Query: "苹果", TopK: 5},
	})
	if err != nil || len(out.Chunks) != 0 {
		t.Fatalf("删除后检索: %+v err=%v", out, err)
	}
	// 重复删除幂等：集合内无该文档的块，Store 成功
	if err := store.DeleteByFilter(ctx, testCollection, map[string]string{MetadataKeyDocumentID: "doc"}); err != nil {
		t.Fatalf("重复删除应幂等: %v", err)
	}
}

// TestCollectionValidation 覆盖两条流程的集合名校验
func TestCollectionValidation(t *testing.T) {
	t.Parallel()
	e, emb, _ := newTestEngine(t)
	ctx := context.Background()
	if _, err := e.Index(ctx, "", IndexRequest{Documents: []Document{
		{ID: "doc", Content: "x"},
	}}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("空集合名: %v", err)
	}
	if _, err := e.Index(ctx, "bad-name", IndexRequest{Documents: []Document{
		{ID: "doc", Content: "x"},
	}}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("非法集合名: %v", err)
	}
	if _, err := e.Retrieve(ctx, "bad-name", RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{Query: "x", TopK: 5},
	}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("非法集合名: %v", err)
	}
	if emb.callCount() != 0 {
		t.Fatalf("非法集合名不应向量化，实际 %d 次", emb.callCount())
	}
}

// TestNewAssemblesDependencies 覆盖构造装配与门面接口
func TestNewAssemblesDependencies(t *testing.T) {
	t.Parallel()
	store := newTestStore(t, testDimension)
	emb := &fakeEmbedder{dim: testDimension}
	e := newTestEngineWithDeps(t, emb, store)
	if e.chunker == nil || e.embedder == nil || e.retriever == nil ||
		e.reranker == nil || e.generator == nil || e.store == nil {
		t.Fatal("构造依赖未装配完整")
	}
	if _, ok := any(e).(Engine); !ok {
		t.Fatal("engine 未实现 Engine 门面接口")
	}
}

// TestNewInvalidConfig 覆盖构造配置全量校验
func TestNewInvalidConfig(t *testing.T) {
	t.Parallel()
	store := newTestStore(t, testDimension)
	emb := &fakeEmbedder{dim: testDimension}
	full := func() Config {
		return Config{
			Chunker:   mustTestChunker(t),
			Embedder:  emb,
			Retriever: newTestRetriever(t, emb, store),
			Reranker:  rerank.NewPassThrough(),
			Generator: generation.NewBasic(),
			Store:     store,
		}
	}
	cases := []struct {
		name string
		cfg  Config
	}{
		{"nil chunker", func() Config { c := full(); c.Chunker = nil; return c }()},
		{"nil embedder", func() Config { c := full(); c.Embedder = nil; return c }()},
		{"nil retriever", func() Config { c := full(); c.Retriever = nil; return c }()},
		{"nil reranker", func() Config { c := full(); c.Reranker = nil; return c }()},
		{"nil generator", func() Config { c := full(); c.Generator = nil; return c }()},
		{"nil store", func() Config { c := full(); c.Store = nil; return c }()},
		{"nil all", Config{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.cfg); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("期望 ErrInvalidArgument，实际 %v", err)
			}
		})
	}
}

// TestMustNewPanics 覆盖 Fail Fast 构造
func TestMustNewPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("MustNew 应 panic")
		}
	}()
	MustNew(Config{})
}

// TestIndexInvalidRequests 覆盖入库入参校验且不产生副作用
func TestIndexInvalidRequests(t *testing.T) {
	t.Parallel()
	e, emb, _ := newTestEngine(t)
	ctx := context.Background()
	cases := []struct {
		name string
		req  IndexRequest
	}{
		{"empty documents", IndexRequest{}},
		{"empty id", IndexRequest{Documents: []Document{{Content: "x"}}}},
		{"empty content", IndexRequest{Documents: []Document{{ID: "doc", Content: " \n "}}}},
		{"duplicate id", IndexRequest{Documents: []Document{
			{ID: "doc", Content: "a"}, {ID: "doc", Content: "b"},
		}}},
		{"long id", IndexRequest{Documents: []Document{{
			ID: strings.Repeat("a", maxDocIDLen+1), Content: "x",
		}}}},
		{"long multibyte id", IndexRequest{Documents: []Document{{
			ID: strings.Repeat("中", maxDocIDLen), Content: "x",
		}}}},
		{"reserved metadata", IndexRequest{Documents: []Document{{
			ID: "doc", Content: "x", Metadata: map[string]string{MetadataKeyContent: "x"},
		}}}},
		{"reserved chunk count", IndexRequest{Documents: []Document{{
			ID: "doc", Content: "x", Metadata: map[string]string{MetadataKeyChunkCount: "1"},
		}}}},
		{"empty metadata key", IndexRequest{Documents: []Document{{
			ID: "doc", Content: "x", Metadata: map[string]string{" ": "x"},
		}}}},
		{"bad metadata key", IndexRequest{Documents: []Document{{
			ID: "doc", Content: "x", Metadata: map[string]string{`a"b`: "x"},
		}}}},
		{"duplicate normalized key", IndexRequest{Documents: []Document{{
			ID: "doc", Content: "x", Metadata: map[string]string{"k": "1", " k ": "2"},
		}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := e.Index(ctx, testCollection, tc.req); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("期望 ErrInvalidArgument，实际 %v", err)
			}
		})
	}
	has, err := e.store.HasCollection(ctx, testCollection)
	if err != nil || has {
		t.Fatalf("非法请求不应创建集合: has=%v err=%v", has, err)
	}
	if emb.callCount() != 0 {
		t.Fatalf("非法请求不应向量化，实际 %d 次", emb.callCount())
	}
}

// TestRetrieveInvalidRequests 覆盖检索入参校验
func TestRetrieveInvalidRequests(t *testing.T) {
	t.Parallel()
	e, emb, _ := newTestEngine(t)
	ctx := context.Background()
	cases := []struct {
		name string
		req  RetrieveRequest
	}{
		{"empty query", RetrieveRequest{RetrieveRequest: retrieval.RetrieveRequest{Query: "  ", TopK: 5}}},
		{"zero topk", RetrieveRequest{RetrieveRequest: retrieval.RetrieveRequest{Query: "苹果"}}},
		{"negative topk", RetrieveRequest{RetrieveRequest: retrieval.RetrieveRequest{Query: "苹果", TopK: -1}}},
		{"topk too large", RetrieveRequest{RetrieveRequest: retrieval.RetrieveRequest{Query: "苹果", TopK: vectordb.MaxTopK + 1}}},
		{"nan min score", RetrieveRequest{RetrieveRequest: retrieval.RetrieveRequest{Query: "苹果", TopK: 5, MinScore: float32(math.NaN())}}},
		{"empty filter key", RetrieveRequest{RetrieveRequest: retrieval.RetrieveRequest{Query: "苹果", TopK: 5, Filter: map[string]string{"": "x"}}}},
		{"bad filter key", RetrieveRequest{RetrieveRequest: retrieval.RetrieveRequest{Query: "苹果", TopK: 5, Filter: map[string]string{`a"b`: "x"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := e.Retrieve(ctx, testCollection, tc.req); !errors.Is(err, retrieval.ErrInvalidArgument) {
				t.Fatalf("期望 retrieval.ErrInvalidArgument，实际 %v", err)
			}
		})
	}
	// 负 RerankTopN 是本包编排层校验
	if _, err := e.Retrieve(ctx, testCollection, RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{Query: "苹果", TopK: 5},
		RerankTopN:      -1,
	}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("负 RerankTopN: 期望 ErrInvalidArgument，实际 %v", err)
	}
	if emb.callCount() != 0 {
		t.Fatalf("非法请求不应向量化，实际 %d 次", emb.callCount())
	}
}

// TestDimensionValidatedByStore 覆盖维度一致性由 vectordb 校验：写路径与检索路径一致
func TestDimensionValidatedByStore(t *testing.T) {
	t.Parallel()
	store := newTestStore(t, testDimension)
	emb := &fakeEmbedder{dim: testDimension}
	e := newTestEngineWithDeps(t, emb, store)
	ctx := context.Background()
	if _, err := e.Index(ctx, "rag_dim", IndexRequest{Documents: []Document{{ID: "doc", Content: "苹果"}}}); err != nil {
		t.Fatal(err)
	}
	emb.setDim(testDimension - 1)
	// 写路径：记录维度与集合维度不符由 vectordb 校验拒绝
	if _, err := e.Index(ctx, "rag_dim", IndexRequest{Documents: []Document{{ID: "doc2", Content: "苹果"}}}); !errors.Is(err, vectordb.ErrInvalidArgument) {
		t.Fatalf("写路径维度不符应透传 vectordb 校验，实际 %v", err)
	}
	// 检索路径：查询向量维度不符同样由 vectordb 校验兜底
	if _, err := e.Retrieve(ctx, "rag_dim", RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{Query: "苹果", TopK: 5},
	}); !errors.Is(err, vectordb.ErrInvalidArgument) {
		t.Fatalf("检索路径维度不符应透传 vectordb 校验，实际 %v", err)
	}
}

// TestIndexEmbedError 覆盖向量化错误原样透传
func TestIndexEmbedError(t *testing.T) {
	t.Parallel()
	store := newTestStore(t, testDimension)
	e := newTestEngineWithDeps(t, &fakeEmbedder{dim: testDimension, err: embedding.ErrRateLimited}, store)
	_, err := e.Index(context.Background(), "rag_embed_err", IndexRequest{Documents: []Document{
		{ID: "doc", Content: "苹果"},
	}})
	if !errors.Is(err, embedding.ErrRateLimited) {
		t.Fatalf("期望 embedding.ErrRateLimited，实际 %v", err)
	}
}

// TestIndexSingleEmbedCall 覆盖向量化整批委托：分批由 embedding 内部承担
func TestIndexSingleEmbedCall(t *testing.T) {
	t.Parallel()
	e, emb, _ := newTestEngine(t)
	total := 300
	chunks := make([]string, total)
	for i := range chunks {
		chunks[i] = "苹果"
	}
	res, err := e.Index(context.Background(), testCollection, IndexRequest{
		Documents: []Document{{ID: "doc", Content: "原始文本"}},
		Chunker:   &stubChunker{chunks: chunks},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Chunks != total || res.Tokens != total {
		t.Fatalf("入库结果: %+v", res)
	}
	// 单篇文档一次 Embed 调用，分批由 embedding 内部承担
	if emb.callCount() != 1 {
		t.Fatalf("调用次数 = %d，期望 1", emb.callCount())
	}
}

// TestRetrieveSkipsForeignChunks 覆盖非本库写入、缺少块文本的命中
func TestRetrieveSkipsForeignChunks(t *testing.T) {
	t.Parallel()
	e, _, store := newTestEngine(t)
	ctx := context.Background()
	if err := store.CreateCollection(ctx, testCollection); err != nil {
		t.Fatal(err)
	}
	if err := store.Upsert(ctx, testCollection, []vectordb.Record{
		{ID: "foreign", Vector: []float32{1, 0, 0}, Metadata: map[string]string{"foo": "bar"}},
	}); err != nil {
		t.Fatal(err)
	}
	out, err := e.Retrieve(ctx, testCollection, RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{Query: "苹果", TopK: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Chunks) != 0 {
		t.Fatalf("缺少块文本的命中应被忽略: %+v", out.Chunks)
	}
}

// TestIndexCustomChunker 覆盖按请求注入自定义切块策略
func TestIndexCustomChunker(t *testing.T) {
	t.Parallel()
	e, _, _ := newTestEngine(t)
	custom := &stubChunker{chunks: []string{"自定义块一", "自定义块二"}}
	res, err := e.Index(context.Background(), testCollection, IndexRequest{
		Documents: []Document{{ID: "doc", Content: "原始文本"}},
		Chunker:   custom,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Chunks != 2 || custom.calls != 1 {
		t.Fatalf("自定义切块结果: %+v calls=%d", res, custom.calls)
	}
	out, err := e.Retrieve(context.Background(), testCollection, RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{Query: "自定义", TopK: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Chunks) != 2 || out.Chunks[0].Content != "自定义块一" || out.Chunks[1].Content != "自定义块二" {
		t.Fatalf("自定义切块内容: %+v", out.Chunks)
	}
}

// TestIndexCustomChunkerInvalid 覆盖自定义切块输出校验
func TestIndexCustomChunkerInvalid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		chunker *stubChunker
		want    error
	}{
		{"empty", &stubChunker{}, chunking.ErrInvalidArgument},
		{"blank", &stubChunker{chunks: []string{"  "}}, chunking.ErrInvalidArgument},
		{"too large", &stubChunker{chunks: []string{strings.Repeat("a", chunking.MaxChunkSize+1)}}, chunking.ErrInvalidArgument},
		{"split error", &stubChunker{err: embedding.ErrRateLimited}, embedding.ErrRateLimited},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, _, _ := newTestEngine(t)
			_, err := e.Index(context.Background(), testCollection, IndexRequest{
				Documents: []Document{{ID: "doc", Content: "原始文本"}},
				Chunker:   tc.chunker,
			})
			if !errors.Is(err, tc.want) {
				t.Fatalf("期望 %v，实际 %v", tc.want, err)
			}
		})
	}
}

// TestRetrieveRerankReordersAndScores 覆盖精重排：顺序、双分数口径与拼接上下文顺序
func TestRetrieveRerankReordersAndScores(t *testing.T) {
	t.Parallel()
	store := newTestStore(t, testDimension)
	emb := &fakeEmbedder{dim: testDimension}
	rr := &fakeReranker{score: func(_ int, doc string) float32 {
		if strings.Contains(doc, "退款") {
			return 0.9
		}
		return 0.1
	}}
	gen := &fakeGenerator{}
	e := newTestEngineWithConfig(t, Config{
		Chunker:   mustTestChunker(t),
		Embedder:  emb,
		Retriever: newTestRetriever(t, emb, store),
		Reranker:  rr,
		Generator: gen,
		Store:     store,
	})
	ctx := context.Background()
	if _, err := e.Index(ctx, testCollection, IndexRequest{Documents: []Document{
		// 纯向量策略下两文档正交 0 分按 ID 升序：doc-a 在前、doc-z 在后
		{ID: "doc-a", Content: "香蕉。配送范围说明。"},
		{ID: "doc-z", Content: "苹果。退款政策说明。"},
	}}); err != nil {
		t.Fatal(err)
	}
	out, err := e.Retrieve(ctx, testCollection, RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{
			Query: "如何退款", Strategy: retrieval.StrategyVector, TopK: 2,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Chunks) != 2 || out.Chunks[0].DocumentID != "doc-z" || out.Chunks[1].DocumentID != "doc-a" {
		t.Fatalf("重排顺序错误: %+v", out.Chunks)
	}
	// RerankScore 是重排分，Score 保留检索分
	if out.Chunks[0].RerankScore != 0.9 || out.Chunks[1].RerankScore != 0.1 {
		t.Fatalf("重排分数错误: %+v", out.Chunks)
	}
	if out.Chunks[0].Score != 0 || out.Chunks[1].Score != 0 {
		t.Fatalf("检索分应保留原值: %+v", out.Chunks)
	}
	// 拼接收到的是重排后的上下文与修剪后的问题
	if gen.calls != 1 {
		t.Fatalf("拼接调用 = %d", gen.calls)
	}
	if gen.req.Query != "如何退款" {
		t.Fatalf("拼接 Query = %q", gen.req.Query)
	}
	if len(gen.req.Contexts) != 2 ||
		gen.req.Contexts[0].Content != "苹果。退款政策说明。" || gen.req.Contexts[0].Source != "" ||
		gen.req.Contexts[1].Content != "香蕉。配送范围说明。" {
		t.Fatalf("拼接上下文顺序错误: %+v", gen.req.Contexts)
	}
	if out.Prompt != "测试提示词" {
		t.Fatalf("Prompt = %q", out.Prompt)
	}
}

// TestRetrieveRerankTopN 覆盖重排后保留条数
func TestRetrieveRerankTopN(t *testing.T) {
	t.Parallel()
	e, _, _ := newTestEngine(t)
	ctx := context.Background()
	if _, err := e.Index(ctx, testCollection, IndexRequest{Documents: []Document{
		{ID: "doc-a", Content: "苹果是一种水果。"},
		{ID: "doc-b", Content: "苹果富含维生素。"},
	}}); err != nil {
		t.Fatal(err)
	}
	out, err := e.Retrieve(ctx, testCollection, RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{Query: "苹果", TopK: 10},
		RerankTopN:      1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Chunks) != 1 {
		t.Fatalf("RerankTopN 应截断结果: %+v", out.Chunks)
	}
	if len(out.Prompt) == 0 {
		t.Fatal("Prompt 不应为空")
	}
}

// TestRetrieveRerankSkippedWithoutHits 覆盖无命中时跳过重排
func TestRetrieveRerankSkippedWithoutHits(t *testing.T) {
	t.Parallel()
	store := newTestStore(t, testDimension)
	emb := &fakeEmbedder{dim: testDimension}
	rr := &fakeReranker{}
	e := newTestEngineWithConfig(t, Config{
		Chunker:   mustTestChunker(t),
		Embedder:  emb,
		Retriever: newTestRetriever(t, emb, store),
		Reranker:  rr,
		Generator: generation.NewBasic(),
		Store:     store,
	})
	out, err := e.Retrieve(context.Background(), testCollection, RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{Query: "苹果", TopK: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Chunks) != 0 {
		t.Fatalf("空集合不应有命中: %+v", out.Chunks)
	}
	if rr.calls != 0 {
		t.Fatalf("无命中不应调用重排，实际 %d 次", rr.calls)
	}
	if out.Prompt != "苹果" {
		t.Fatalf("空上下文 Prompt = %q", out.Prompt)
	}
}

// TestRetrieveRerankErrorPassthrough 覆盖重排错误原样透传
func TestRetrieveRerankErrorPassthrough(t *testing.T) {
	t.Parallel()
	store := newTestStore(t, testDimension)
	emb := &fakeEmbedder{dim: testDimension}
	e := newTestEngineWithConfig(t, Config{
		Chunker:   mustTestChunker(t),
		Embedder:  emb,
		Retriever: newTestRetriever(t, emb, store),
		Reranker:  &fakeReranker{err: rerank.ErrBadResponse},
		Generator: generation.NewBasic(),
		Store:     store,
	})
	ctx := context.Background()
	if _, err := e.Index(ctx, testCollection, IndexRequest{Documents: []Document{
		{ID: "doc", Content: "苹果是一种水果。"},
	}}); err != nil {
		t.Fatal(err)
	}
	_, err := e.Retrieve(ctx, testCollection, RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{Query: "苹果", TopK: 5},
	})
	if !errors.Is(err, rerank.ErrBadResponse) {
		t.Fatalf("期望 rerank.ErrBadResponse，实际 %v", err)
	}
}

// TestRetrieveRerankInvalidIndex 覆盖重排结果下标越界的违约校验（由 rerank.Apply 识别）
func TestRetrieveRerankInvalidIndex(t *testing.T) {
	t.Parallel()
	store := newTestStore(t, testDimension)
	emb := &fakeEmbedder{dim: testDimension}
	e := newTestEngineWithConfig(t, Config{
		Chunker:   mustTestChunker(t),
		Embedder:  emb,
		Retriever: newTestRetriever(t, emb, store),
		Reranker: &fakeReranker{resp: &rerank.RerankResponse{Results: []rerank.RankedDoc{
			{Index: 5, Score: 1},
		}}},
		Generator: generation.NewBasic(),
		Store:     store,
	})
	ctx := context.Background()
	if _, err := e.Index(ctx, testCollection, IndexRequest{Documents: []Document{
		{ID: "doc", Content: "苹果是一种水果。"},
	}}); err != nil {
		t.Fatal(err)
	}
	_, err := e.Retrieve(ctx, testCollection, RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{Query: "苹果", TopK: 5},
	})
	if !errors.Is(err, rerank.ErrBadResponse) {
		t.Fatalf("期望 rerank.ErrBadResponse，实际 %v", err)
	}
}

// TestRetrieveGeneratorErrorPassthrough 覆盖拼接错误原样透传
func TestRetrieveGeneratorErrorPassthrough(t *testing.T) {
	t.Parallel()
	store := newTestStore(t, testDimension)
	emb := &fakeEmbedder{dim: testDimension}
	e := newTestEngineWithConfig(t, Config{
		Chunker:   mustTestChunker(t),
		Embedder:  emb,
		Retriever: newTestRetriever(t, emb, store),
		Reranker:  rerank.NewPassThrough(),
		Generator: &fakeGenerator{err: generation.ErrInvalidArgument},
		Store:     store,
	})
	ctx := context.Background()
	if _, err := e.Index(ctx, testCollection, IndexRequest{Documents: []Document{
		{ID: "doc", Content: "苹果是一种水果。"},
	}}); err != nil {
		t.Fatal(err)
	}
	_, err := e.Retrieve(ctx, testCollection, RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{Query: "苹果", TopK: 5},
	})
	if !errors.Is(err, generation.ErrInvalidArgument) {
		t.Fatalf("期望 generation.ErrInvalidArgument，实际 %v", err)
	}
}

// TestRetrieveGeneratorEmptyPrompt 覆盖拼接返回空提示词的违约校验（由 generation.ValidateResponse 识别）
func TestRetrieveGeneratorEmptyPrompt(t *testing.T) {
	t.Parallel()
	store := newTestStore(t, testDimension)
	emb := &fakeEmbedder{dim: testDimension}
	e := newTestEngineWithConfig(t, Config{
		Chunker:   mustTestChunker(t),
		Embedder:  emb,
		Retriever: newTestRetriever(t, emb, store),
		Reranker:  rerank.NewPassThrough(),
		Generator: &fakeGenerator{prompt: func(generation.GenerateRequest) string { return "  " }},
		Store:     store,
	})
	ctx := context.Background()
	if _, err := e.Index(ctx, testCollection, IndexRequest{Documents: []Document{
		{ID: "doc", Content: "苹果是一种水果。"},
	}}); err != nil {
		t.Fatal(err)
	}
	_, err := e.Retrieve(ctx, testCollection, RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{Query: "苹果", TopK: 5},
	})
	if !errors.Is(err, generation.ErrBadResponse) {
		t.Fatalf("期望 generation.ErrBadResponse，实际 %v", err)
	}
}

// flakyStore 在前 N 次 ReplaceByFilter 调用上返回错误，用于模拟写入瞬时故障
type flakyStore struct {
	vectordb.Store
	replaceFailures int
}

// ReplaceByFilter 按剩余失败次数返回错误，其余透传
func (s *flakyStore) ReplaceByFilter(ctx context.Context, collection string, filter map[string]string, records []vectordb.Record) error {
	if s.replaceFailures > 0 {
		s.replaceFailures--
		return errors.New("simulated replace failure")
	}
	return s.Store.ReplaceByFilter(ctx, collection, filter, records)
}

// TestIndexReplaceFailureKeepsOldVersionIntact 验证替换失败时旧版本保持完整，重试后只剩新版本
func TestIndexReplaceFailureKeepsOldVersionIntact(t *testing.T) {
	t.Parallel()
	store := newTestStore(t, testDimension)
	flaky := &flakyStore{Store: store}
	e := newTestEngineWithDeps(t, &fakeEmbedder{dim: testDimension}, flaky)
	ctx := context.Background()
	large := &stubChunker{chunks: []string{"a0", "a1", "a2", "a3", "a4", "a5"}}
	if _, err := e.Index(ctx, testCollection, IndexRequest{
		Documents: []Document{{ID: "doc", Content: "large"}},
		Chunker:   large,
	}); err != nil {
		t.Fatal(err)
	}
	small := &stubChunker{chunks: []string{"b0"}}
	flaky.replaceFailures = 1
	if _, err := e.Index(ctx, testCollection, IndexRequest{
		Documents: []Document{{ID: "doc", Content: "small"}},
		Chunker:   small,
	}); err == nil {
		t.Fatal("期望替换失败")
	}
	// 替换失败时旧版本保持完整
	got, err := store.Get(ctx, testCollection, []string{chunkID("doc", 0), chunkID("doc", 5)})
	if err != nil || len(got) != 2 ||
		got[0].Metadata[MetadataKeyContent] != "a0" || got[1].Metadata[MetadataKeyContent] != "a5" {
		t.Fatalf("旧版本应保持完整: %+v err=%v", got, err)
	}
	// 重试后只保留新版本
	if _, err := e.Index(ctx, testCollection, IndexRequest{
		Documents: []Document{{ID: "doc", Content: "small"}},
		Chunker:   small,
	}); err != nil {
		t.Fatal(err)
	}
	out, err := e.Retrieve(ctx, testCollection, RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{Query: "q", TopK: 20},
	})
	if err != nil || len(out.Chunks) != 1 || out.Chunks[0].Content != "b0" {
		t.Fatalf("重试后应只剩新版本: %+v err=%v", out, err)
	}
}

// TestConcurrentSameDocumentIndexKeepsConsistentChunks 验证并发写入同一文档后块集一致
func TestConcurrentSameDocumentIndexKeepsConsistentChunks(t *testing.T) {
	t.Parallel()
	e, _, store := newTestEngine(t)
	ctx := context.Background()
	large := []string{"a0", "a1", "a2", "a3", "a4", "a5"}
	small := []string{"b0"}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = e.Index(ctx, testCollection, IndexRequest{
				Documents: []Document{{ID: "doc", Content: "large"}},
				Chunker:   &stubChunker{chunks: large},
			})
		}()
		go func() {
			defer wg.Done()
			_, _ = e.Index(ctx, testCollection, IndexRequest{
				Documents: []Document{{ID: "doc", Content: "small"}},
				Chunker:   &stubChunker{chunks: small},
			})
		}()
	}
	wg.Wait()
	ids := make([]string, 6)
	for i := range ids {
		ids[i] = chunkID("doc", i)
	}
	got, err := store.Get(ctx, testCollection, ids)
	if err != nil {
		t.Fatal(err)
	}
	// 并发写入后只能留下某一套完整版本，不能出现新旧块混合
	byID := make(map[string]string, len(got))
	for _, rec := range got {
		byID[rec.ID] = rec.Metadata[MetadataKeyContent]
	}
	switch len(got) {
	case 1:
		if byID[chunkID("doc", 0)] != "b0" {
			t.Fatalf("小版本不完整: %+v", got)
		}
	case 6:
		for i, want := range large {
			if byID[chunkID("doc", i)] != want {
				t.Fatalf("大版本不完整: %+v", got)
			}
		}
	default:
		t.Fatalf("并发写入后块集不完整: %d 块", len(got))
	}
}

// TestEngineConcurrentRetrieve 覆盖并发检索
func TestEngineConcurrentRetrieve(t *testing.T) {
	t.Parallel()
	e, _, _ := newTestEngine(t)
	ctx := context.Background()
	if _, err := e.Index(ctx, testCollection, IndexRequest{Documents: []Document{
		{ID: "doc", Content: "苹果是一种水果。"},
	}}); err != nil {
		t.Fatal(err)
	}
	const goroutines = 8
	const rounds = 10
	errs := make(chan error, goroutines)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < rounds; j++ {
				out, err := e.Retrieve(context.Background(), testCollection, RetrieveRequest{
					RetrieveRequest: retrieval.RetrieveRequest{Query: "苹果", TopK: 5},
				})
				if err != nil {
					errs <- err
					return
				}
				if len(out.Chunks) == 0 {
					errs <- errors.New("并发检索结果为空")
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

// TestRetrieveHybridLexicalBoost 覆盖混合检索词法命中提升排名
func TestRetrieveHybridLexicalBoost(t *testing.T) {
	t.Parallel()
	e, _, _ := newTestEngine(t)
	ctx := context.Background()
	if _, err := e.Index(ctx, testCollection, IndexRequest{Documents: []Document{
		// fakeEmbedder 把 苹果/香蕉 映到不同维度；doc-z 含查询词但按 ID 排在 doc-a 之后
		{ID: "doc-a", Content: "香蕉。配送范围说明。"},
		{ID: "doc-z", Content: "苹果。退款政策说明。"},
	}}); err != nil {
		t.Fatal(err)
	}
	// 查询"如何退款"的向量与两文档正交（落第三维），纯向量两文档并列 0 分按 ID 升序；
	// 混合检索下 doc-z 靠 BM25 词法命中跃居第一
	out, err := e.Retrieve(ctx, testCollection, RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{Query: "如何退款", TopK: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Chunks) != 2 || out.Chunks[0].DocumentID != "doc-z" {
		t.Fatalf("词法命中应排第一: %+v", out.Chunks)
	}
	if out.Chunks[0].Score <= out.Chunks[1].Score {
		t.Fatalf("混合排序错误: %+v", out.Chunks)
	}
	// 纯向量策略不启用 BM25：两文档并列 0 分，按 ID 升序 doc-a 在前
	out, err = e.Retrieve(ctx, testCollection, RetrieveRequest{
		RetrieveRequest: retrieval.RetrieveRequest{
			Query: "如何退款", Strategy: retrieval.StrategyVector, TopK: 2,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Chunks) != 2 || out.Chunks[0].DocumentID != "doc-a" || out.Chunks[0].Score != 0 || out.Chunks[1].Score != 0 {
		t.Fatalf("纯向量应为无区分的正交得分: %+v", out.Chunks)
	}
}
