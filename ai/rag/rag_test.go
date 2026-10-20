// Package rag Engine 主流程测试。
//
// @author zwei
package rag

import (
	"context"
	"math"
	"strings"
	"sync"
	"testing"

	"gitee.com/hq_project/yomi-boot/ai/embedding"
	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/vectordb"
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

// stubSplitter 返回固定块，用于验证自定义策略注入
type stubSplitter struct {
	chunks []string
	err    error
	calls  int
}

// Split 实现 Splitter
func (s *stubSplitter) Split(text string) ([]string, error) {
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

// newTestEngine 创建默认测试引擎
func newTestEngine(t *testing.T) (*Engine, *fakeEmbedder, vectordb.Store) {
	t.Helper()
	store := newTestStore(t, testDimension)
	emb := &fakeEmbedder{dim: testDimension}
	e, err := New(Config{Embedder: emb, Store: store})
	if err != nil {
		t.Fatal(err)
	}
	return e, emb, store
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
	chunks, err := e.Retrieve(ctx, testCollection, RetrieveRequest{Query: "苹果", TopK: 10})
	if err != nil {
		t.Fatal(err)
	}
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
	if emb.callCount() != 2 {
		t.Fatalf("向量化调用 = %d，期望 2", emb.callCount())
	}

	chunks, err = e.Retrieve(ctx, testCollection, RetrieveRequest{
		Query:  "水果",
		TopK:   10,
		Filter: map[string]string{MetadataKeySource: "fruit/banana.md"},
	})
	if err != nil || len(chunks) != 1 || chunks[0].DocumentID != "doc-banana" {
		t.Fatalf("来源过滤: %+v err=%v", chunks, err)
	}
	chunks, err = e.Retrieve(ctx, testCollection, RetrieveRequest{
		Query:  "水果",
		TopK:   10,
		Filter: map[string]string{"lang": "zh"},
	})
	if err != nil || len(chunks) != 1 || chunks[0].DocumentID != "doc-apple" {
		t.Fatalf("自定义元数据过滤: %+v err=%v", chunks, err)
	}
	chunks, err = e.Retrieve(ctx, testCollection, RetrieveRequest{Query: "苹果", TopK: 10, MinScore: 0.5})
	if err != nil || len(chunks) != 1 || chunks[0].DocumentID != "doc-apple" {
		t.Fatalf("阈值过滤: %+v err=%v", chunks, err)
	}
	chunks, err = e.Retrieve(ctx, testCollection, RetrieveRequest{Query: "苹果", TopK: 10})
	if err != nil || len(chunks) != 2 {
		t.Fatalf("不设阈值: %+v err=%v", chunks, err)
	}
	chunks, err = e.Retrieve(ctx, testCollection, RetrieveRequest{Query: "水果", TopK: 1})
	if err != nil || len(chunks) != 1 {
		t.Fatalf("TopK 生效: %+v err=%v", chunks, err)
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
	sales, err := e.Retrieve(ctx, "kb_sales", RetrieveRequest{Query: "香蕉", TopK: 10, MinScore: 0.5})
	if err != nil || len(sales) != 0 {
		t.Fatalf("销售库不应命中人事文档: %+v err=%v", sales, err)
	}
	hr, err := e.Retrieve(ctx, "kb_hr", RetrieveRequest{Query: "香蕉", TopK: 10, MinScore: 0.5})
	if err != nil || len(hr) != 1 || hr[0].DocumentID != "hr-1" {
		t.Fatalf("人事库命中: %+v err=%v", hr, err)
	}
	if err := e.DeleteDocument(ctx, "kb_sales", "sales-1"); err != nil {
		t.Fatal(err)
	}
	hr, err = e.Retrieve(ctx, "kb_hr", RetrieveRequest{Query: "香蕉", TopK: 10, MinScore: 0.5})
	if err != nil || len(hr) != 1 {
		t.Fatalf("删除销售库文档不应影响人事库: %+v err=%v", hr, err)
	}
	if err := e.DropCollection(ctx, "kb_sales"); err != nil {
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
	chunks, err := e.Retrieve(context.Background(), testCollection, RetrieveRequest{Query: "苹果", TopK: 5})
	if err != nil {
		t.Fatal(err)
	}
	if chunks == nil || len(chunks) != 0 {
		t.Fatalf("空检索应返回非 nil 空切片: %#v", chunks)
	}
}

// TestIndexReplacesStaleChunks 覆盖重复入库时清理多余旧块
func TestIndexReplacesStaleChunks(t *testing.T) {
	t.Parallel()
	e, _, store := newTestEngine(t)
	splitter, err := NewSplitter(SplitConfig{ChunkSize: 6, ChunkOverlap: 1})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	res, err := e.Index(ctx, testCollection, IndexRequest{
		Documents: []Document{{ID: "doc", Content: "苹果。苹果。苹果。苹果。"}},
		Splitter:  splitter,
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
		Splitter:  splitter,
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
	chunks, err := e.Retrieve(ctx, testCollection, RetrieveRequest{Query: "苹果", TopK: 20})
	if err != nil || len(chunks) != 1 || chunks[0].DocumentID != "doc" {
		t.Fatalf("重复入库后检索: %+v err=%v", chunks, err)
	}
}

// TestDeleteDocument 覆盖按文档删除与幂等语义
func TestDeleteDocument(t *testing.T) {
	t.Parallel()
	e, _, _ := newTestEngine(t)
	ctx := context.Background()
	if err := e.DeleteDocument(ctx, testCollection, "missing"); err != nil {
		t.Fatalf("集合不存在时应幂等: %v", err)
	}
	if err := e.DeleteDocument(ctx, testCollection, " "); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("非法文档 ID: %v", err)
	}
	if _, err := e.Index(ctx, testCollection, IndexRequest{Documents: []Document{
		{ID: "doc", Content: "苹果是一种水果。"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := e.DeleteDocument(ctx, testCollection, "doc"); err != nil {
		t.Fatal(err)
	}
	chunks, err := e.Retrieve(ctx, testCollection, RetrieveRequest{Query: "苹果", TopK: 5})
	if err != nil || len(chunks) != 0 {
		t.Fatalf("删除后检索: %+v err=%v", chunks, err)
	}
	if err := e.DeleteDocument(ctx, testCollection, "doc"); err != nil {
		t.Fatalf("重复删除应幂等: %v", err)
	}
}

// TestCollectionLifecycle 覆盖集合创建与删除的幂等
func TestCollectionLifecycle(t *testing.T) {
	t.Parallel()
	e, _, _ := newTestEngine(t)
	ctx := context.Background()
	if err := e.EnsureCollection(ctx, testCollection); err != nil {
		t.Fatal(err)
	}
	has, err := e.store.HasCollection(ctx, testCollection)
	if err != nil || !has {
		t.Fatalf("集合未创建: has=%v err=%v", has, err)
	}
	if err := e.EnsureCollection(ctx, testCollection); err != nil {
		t.Fatalf("重复创建: %v", err)
	}
	if err := e.DropCollection(ctx, testCollection); err != nil {
		t.Fatal(err)
	}
	has, err = e.store.HasCollection(ctx, testCollection)
	if err != nil || has {
		t.Fatalf("集合未删除: has=%v err=%v", has, err)
	}
	if err := e.DropCollection(ctx, testCollection); err != nil {
		t.Fatalf("重复删除: %v", err)
	}
}

// TestCollectionValidation 覆盖集合名校验
func TestCollectionValidation(t *testing.T) {
	t.Parallel()
	e, emb, _ := newTestEngine(t)
	ctx := context.Background()
	if _, err := e.Index(ctx, "", IndexRequest{Documents: []Document{
		{ID: "doc", Content: "x"},
	}}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("空集合名: %v", err)
	}
	if _, err := e.Retrieve(ctx, "bad-name", RetrieveRequest{Query: "x", TopK: 5}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("非法集合名: %v", err)
	}
	if err := e.DeleteDocument(ctx, "1bad", "doc"); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("非法集合名: %v", err)
	}
	if err := e.EnsureCollection(ctx, "  "); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("空白集合名: %v", err)
	}
	if err := e.DropCollection(ctx, "bad-name"); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("非法集合名: %v", err)
	}
	if emb.callCount() != 0 {
		t.Fatalf("非法集合名不应向量化，实际 %d 次", emb.callCount())
	}
}

// TestNewMinimal 覆盖最小构造配置与内部默认切块器
func TestNewMinimal(t *testing.T) {
	t.Parallel()
	store := newTestStore(t, testDimension)
	e, err := New(Config{Embedder: &fakeEmbedder{dim: testDimension}, Store: store})
	if err != nil {
		t.Fatal(err)
	}
	recursive, ok := e.defaultSplitter.(*recursiveSplitter)
	if !ok {
		t.Fatalf("默认切块策略 = %T", e.defaultSplitter)
	}
	if recursive.chunkSize != defaultChunkSize || recursive.chunkOverlap != defaultChunkOverlap {
		t.Fatalf("默认切块参数: %+v", recursive)
	}
	if e.dimension.Load() != 0 {
		t.Fatalf("维度应延迟锁定: %d", e.dimension.Load())
	}
}

// TestNewInvalidConfig 覆盖最小构造配置校验
func TestNewInvalidConfig(t *testing.T) {
	t.Parallel()
	store := newTestStore(t, testDimension)
	emb := &fakeEmbedder{dim: testDimension}
	cases := []struct {
		name string
		cfg  Config
	}{
		{"nil embedder", Config{Store: store}},
		{"nil store", Config{Embedder: emb}},
		{"nil both", Config{}},
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
		{"reserved metadata", IndexRequest{Documents: []Document{{
			ID: "doc", Content: "x", Metadata: map[string]string{MetadataKeyContent: "x"},
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
		{"empty query", RetrieveRequest{Query: "  ", TopK: 5}},
		{"zero topk", RetrieveRequest{Query: "苹果"}},
		{"negative topk", RetrieveRequest{Query: "苹果", TopK: -1}},
		{"topk too large", RetrieveRequest{Query: "苹果", TopK: maxTopK + 1}},
		{"nan min score", RetrieveRequest{Query: "苹果", TopK: 5, MinScore: float32(math.NaN())}},
		{"empty filter key", RetrieveRequest{Query: "苹果", TopK: 5, Filter: map[string]string{"": "x"}}},
		{"bad filter key", RetrieveRequest{Query: "苹果", TopK: 5, Filter: map[string]string{`a"b`: "x"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := e.Retrieve(ctx, testCollection, tc.req); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("期望 ErrInvalidArgument，实际 %v", err)
			}
		})
	}
	if emb.callCount() != 0 {
		t.Fatalf("非法请求不应向量化，实际 %d 次", emb.callCount())
	}
}

// TestDimensionLock 覆盖首次向量化后锁定维度
func TestDimensionLock(t *testing.T) {
	t.Parallel()
	store := newTestStore(t, testDimension)
	emb := &fakeEmbedder{dim: testDimension}
	e, err := New(Config{Embedder: emb, Store: store})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := e.Retrieve(ctx, "rag_dim", RetrieveRequest{Query: "苹果", TopK: 5}); err != nil {
		t.Fatal(err)
	}
	emb.setDim(testDimension - 1)
	if _, err := e.Retrieve(ctx, "rag_dim", RetrieveRequest{Query: "苹果", TopK: 5}); !errors.Is(err, ErrDimensionMismatch) {
		t.Fatalf("期望 ErrDimensionMismatch，实际 %v", err)
	}
}

// TestIndexEmbedError 覆盖向量化错误原样透传
func TestIndexEmbedError(t *testing.T) {
	t.Parallel()
	store := newTestStore(t, testDimension)
	e, err := New(Config{
		Embedder: &fakeEmbedder{dim: testDimension, err: embedding.ErrRateLimited},
		Store:    store,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.Index(context.Background(), "rag_embed_err", IndexRequest{Documents: []Document{
		{ID: "doc", Content: "苹果"},
	}})
	if !errors.Is(err, embedding.ErrRateLimited) {
		t.Fatalf("期望 embedding.ErrRateLimited，实际 %v", err)
	}
}

// TestIndexBatchEmbed 覆盖内部分批向量化与用量累计
func TestIndexBatchEmbed(t *testing.T) {
	t.Parallel()
	e, emb, _ := newTestEngine(t)
	total := embedBatchSize + 6
	chunks := make([]string, total)
	for i := range chunks {
		chunks[i] = "苹果"
	}
	res, err := e.Index(context.Background(), testCollection, IndexRequest{
		Documents: []Document{{ID: "doc", Content: "原始文本"}},
		Splitter:  &stubSplitter{chunks: chunks},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Chunks != total || res.Tokens != total {
		t.Fatalf("入库结果: %+v", res)
	}
	if emb.callCount() != 2 {
		t.Fatalf("调用次数 = %d，期望 2", emb.callCount())
	}
}

// TestRetrieveSkipsForeignChunks 覆盖非本库写入、缺少块文本的命中
func TestRetrieveSkipsForeignChunks(t *testing.T) {
	t.Parallel()
	e, _, store := newTestEngine(t)
	ctx := context.Background()
	if err := e.EnsureCollection(ctx, testCollection); err != nil {
		t.Fatal(err)
	}
	if err := store.Upsert(ctx, testCollection, []vectordb.Record{
		{ID: "foreign", Vector: []float32{1, 0, 0}, Metadata: map[string]string{"foo": "bar"}},
	}); err != nil {
		t.Fatal(err)
	}
	chunks, err := e.Retrieve(ctx, testCollection, RetrieveRequest{Query: "苹果", TopK: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 0 {
		t.Fatalf("缺少块文本的命中应被忽略: %+v", chunks)
	}
}

// TestIndexCustomSplitter 覆盖按请求注入自定义切块策略
func TestIndexCustomSplitter(t *testing.T) {
	t.Parallel()
	e, _, _ := newTestEngine(t)
	custom := &stubSplitter{chunks: []string{"自定义块一", "自定义块二"}}
	res, err := e.Index(context.Background(), testCollection, IndexRequest{
		Documents: []Document{{ID: "doc", Content: "原始文本"}},
		Splitter:  custom,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Chunks != 2 || custom.calls != 1 {
		t.Fatalf("自定义切块结果: %+v calls=%d", res, custom.calls)
	}
	chunks, err := e.Retrieve(context.Background(), testCollection, RetrieveRequest{Query: "自定义", TopK: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 || chunks[0].Content != "自定义块一" || chunks[1].Content != "自定义块二" {
		t.Fatalf("自定义切块内容: %+v", chunks)
	}
}

// TestIndexCustomSplitterInvalid 覆盖自定义切块输出校验
func TestIndexCustomSplitterInvalid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		splitter *stubSplitter
		want     error
	}{
		{"empty", &stubSplitter{}, ErrInvalidArgument},
		{"blank", &stubSplitter{chunks: []string{"  "}}, ErrInvalidArgument},
		{"too large", &stubSplitter{chunks: []string{strings.Repeat("a", maxChunkSize+1)}}, ErrInvalidArgument},
		{"split error", &stubSplitter{err: embedding.ErrRateLimited}, embedding.ErrRateLimited},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, _, _ := newTestEngine(t)
			_, err := e.Index(context.Background(), testCollection, IndexRequest{
				Documents: []Document{{ID: "doc", Content: "原始文本"}},
				Splitter:  tc.splitter,
			})
			if !errors.Is(err, tc.want) {
				t.Fatalf("期望 %v，实际 %v", tc.want, err)
			}
		})
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
				chunks, err := e.Retrieve(context.Background(), testCollection, RetrieveRequest{Query: "苹果", TopK: 5})
				if err != nil {
					errs <- err
					return
				}
				if len(chunks) == 0 {
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
