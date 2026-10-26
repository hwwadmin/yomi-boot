// 检索门面测试。
//
// @author zwei

package retrieval

import (
	"context"
	"math"
	"strings"
	"sync"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/ai/embedding"
	"gitee.com/hq_project/yomi-boot/component/ai/vectordb"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

const (
	testCollection = "retrieval_test"
	testDimension  = 3
)

// fakeEmbedder 按关键词返回确定性向量，便于断言检索排序
type fakeEmbedder struct {
	mu    sync.Mutex
	dim   int
	calls int
}

// Embed 实现 embedding.Client
func (f *fakeEmbedder) Embed(ctx context.Context, req embedding.Request) (*embedding.Response, error) {
	f.mu.Lock()
	f.calls++
	dim := f.dim
	f.mu.Unlock()
	vectors := make([][]float64, len(req.Inputs))
	for i, input := range req.Inputs {
		vectors[i] = fakeVector(input, dim)
	}
	return &embedding.Response{
		Model:   "fake-embed",
		Vectors: vectors,
		Usage:   &embedding.TokenUsage{TotalTokens: len(req.Inputs)},
	}, nil
}

// callCount 返回累计向量化次数
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

// captureSearchStore 捕获 Search 入参，用于验证策略传递
type captureSearchStore struct {
	vectordb.Store
	captured *vectordb.SearchQuery
}

// Search 记录入参后透传
func (s captureSearchStore) Search(ctx context.Context, collection string, q vectordb.SearchQuery) ([]vectordb.Hit, error) {
	*s.captured = q
	return s.Store.Search(ctx, collection, q)
}

// newTestEngine 创建挂接 memory store 的测试引擎
func newTestEngine(t *testing.T) (*Engine, *fakeEmbedder, vectordb.Store) {
	t.Helper()
	store, err := vectordb.New(vectordb.Config{Provider: vectordb.ProviderMemory})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	emb := &fakeEmbedder{dim: testDimension}
	e, err := New(Config{Embedder: emb, Store: store})
	if err != nil {
		t.Fatal(err)
	}
	return e, emb, store
}

// TestNewInvalidConfig 覆盖构造配置校验
func TestNewInvalidConfig(t *testing.T) {
	t.Parallel()
	emb := &fakeEmbedder{dim: testDimension}
	store, err := vectordb.New(vectordb.Config{Provider: vectordb.ProviderMemory})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := New(Config{Store: store}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("nil embedder: %v", err)
	}
	if _, err := New(Config{Embedder: emb}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("nil store: %v", err)
	}
	if _, err := New(Config{}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("nil both: %v", err)
	}
}

// TestRetrieveStrategyPlumbing 覆盖检索策略传递：默认与 hybrid 带 BM25 文本，vector 不带
func TestRetrieveStrategyPlumbing(t *testing.T) {
	t.Parallel()
	store, err := vectordb.New(vectordb.Config{Provider: vectordb.ProviderMemory})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	emb := &fakeEmbedder{dim: testDimension}
	captured := &vectordb.SearchQuery{}
	e, err := New(Config{Embedder: emb, Store: captureSearchStore{Store: store, captured: captured}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.CreateCollection(ctx, testCollection, testDimension); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Retrieve(ctx, testCollection, RetrieveRequest{Query: "苹果", TopK: 5, Dimensions: testDimension}); err != nil {
		t.Fatal(err)
	}
	if captured.Text != "苹果" {
		t.Fatalf("默认 hybrid 未传递检索文本: %+v", *captured)
	}
	if _, err := e.Retrieve(ctx, testCollection, RetrieveRequest{
		Query: "苹果", Strategy: StrategyHybrid, TopK: 5, Dimensions: testDimension}); err != nil {
		t.Fatal(err)
	}
	if captured.Text != "苹果" {
		t.Fatalf("hybrid 未传递检索文本: %+v", *captured)
	}
	if _, err := e.Retrieve(ctx, testCollection, RetrieveRequest{
		Query: "苹果", Strategy: StrategyVector, TopK: 5, Dimensions: testDimension}); err != nil {
		t.Fatal(err)
	}
	if captured.Text != "" {
		t.Fatalf("vector 不应传递检索文本: %+v", *captured)
	}
	if _, err := e.Retrieve(ctx, testCollection, RetrieveRequest{
		Query: "苹果", Strategy: "bogus", TopK: 5, Dimensions: testDimension}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("非法策略: %v", err)
	}
}

// TestRetrieveInvalidRequests 覆盖入参校验；非法请求不得消耗向量化
func TestRetrieveInvalidRequests(t *testing.T) {
	t.Parallel()
	e, emb, _ := newTestEngine(t)
	ctx := context.Background()
	cases := []struct {
		name string
		req  RetrieveRequest
	}{
		{"empty query", RetrieveRequest{Query: "  ", TopK: 5, Dimensions: testDimension}},
		{"zero topk", RetrieveRequest{Query: "苹果", Dimensions: testDimension}},
		{"negative topk", RetrieveRequest{Query: "苹果", TopK: -1, Dimensions: testDimension}},
		{"topk too large", RetrieveRequest{Query: "苹果", TopK: vectordb.MaxTopK + 1, Dimensions: testDimension}},
		{"nan min score", RetrieveRequest{Query: "苹果", TopK: 5, MinScore: float32(math.NaN()), Dimensions: testDimension}},
		{"inf min score", RetrieveRequest{Query: "苹果", TopK: 5, MinScore: float32(math.Inf(1)), Dimensions: testDimension}},
		{"empty filter key", RetrieveRequest{Query: "苹果", TopK: 5, Filter: map[string]string{"": "x"}, Dimensions: testDimension}},
		{"bad filter key", RetrieveRequest{Query: "苹果", TopK: 5, Filter: map[string]string{`a"b`: "x"}, Dimensions: testDimension}},
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

// TestRetrieveEmptyCollection 覆盖集合不存在返回空切片且不向量化
func TestRetrieveEmptyCollection(t *testing.T) {
	t.Parallel()
	e, emb, _ := newTestEngine(t)
	hits, err := e.Retrieve(context.Background(), "no_such_coll", RetrieveRequest{Query: "苹果", TopK: 5, Dimensions: testDimension})
	if err != nil {
		t.Fatal(err)
	}
	if hits == nil || len(hits) != 0 {
		t.Fatalf("空检索应返回非 nil 空切片: %#v", hits)
	}
	if emb.callCount() != 0 {
		t.Fatalf("集合不存在不应向量化，实际 %d 次", emb.callCount())
	}
}

// TestRetrieveMinScore 覆盖阈值过滤
func TestRetrieveMinScore(t *testing.T) {
	t.Parallel()
	e, _, _ := newTestEngine(t)
	ctx := context.Background()
	if err := e.store.CreateCollection(ctx, testCollection, testDimension); err != nil {
		t.Fatal(err)
	}
	if err := e.store.Upsert(ctx, testCollection, []vectordb.Record{
		{ID: "near", Vector: []float32{1, 0, 0}},
		{ID: "far", Vector: []float32{0, 0, 1}},
	}); err != nil {
		t.Fatal(err)
	}
	// vector 策略 Score 是余弦相似度：near=1、far=0，0.5 阈值只留 near
	hits, err := e.Retrieve(ctx, testCollection, RetrieveRequest{
		Query: "苹果", Strategy: StrategyVector, TopK: 10, MinScore: 0.5, Dimensions: testDimension})
	if err != nil || len(hits) != 1 || hits[0].ID != "near" {
		t.Fatalf("vector 阈值过滤: %+v err=%v", hits, err)
	}
	// hybrid 策略 Score 是 RRF 融合分，只看两路排名与原始余弦无关：
	// near 为 dense 第 1 名 ≈0.0164，far 为第 2 名 ≈0.0161，阈值按融合分口径设置
	hits, err = e.Retrieve(ctx, testCollection, RetrieveRequest{Query: "苹果", TopK: 10, MinScore: 0.0162, Dimensions: testDimension})
	if err != nil || len(hits) != 1 || hits[0].ID != "near" {
		t.Fatalf("hybrid 阈值过滤: %+v err=%v", hits, err)
	}
	all, err := e.Retrieve(ctx, testCollection, RetrieveRequest{Query: "苹果", TopK: 10, Dimensions: testDimension})
	if err != nil || len(all) != 2 {
		t.Fatalf("不设阈值: %+v err=%v", all, err)
	}
}

// TestRetrieveHybridLexicalBoost 覆盖 memory store 全链路混合检索：词法命中决定头名
func TestRetrieveHybridLexicalBoost(t *testing.T) {
	t.Parallel()
	e, _, _ := newTestEngine(t)
	ctx := context.Background()
	if err := e.store.CreateCollection(ctx, testCollection, testDimension); err != nil {
		t.Fatal(err)
	}
	if err := e.store.Upsert(ctx, testCollection, []vectordb.Record{
		// fakeVector 把 苹果/香蕉 映到不同维度；查询"如何退款"与两记录向量正交，
		// 纯向量并列 0 分按 ID 升序（doc-a 在前），词法命中把 doc-z 抬到第一
		{ID: "doc-a", Vector: []float32{0, 1, 0}, Text: "香蕉。配送范围说明。"},
		{ID: "doc-z", Vector: []float32{1, 0, 0}, Text: "苹果。退款政策说明。"},
	}); err != nil {
		t.Fatal(err)
	}
	hits, err := e.Retrieve(ctx, testCollection, RetrieveRequest{Query: "如何退款", TopK: 2, Dimensions: testDimension})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 || hits[0].ID != "doc-z" || hits[0].Score <= hits[1].Score {
		t.Fatalf("词法命中应排第一: %+v", hits)
	}
}

// fakeHitFilter 后置过滤测试替身
type fakeHitFilter struct {
	keep  func(hit vectordb.Hit) bool
	err   error
	calls int
}

// Filter 实现 HitFilter
func (f *fakeHitFilter) Filter(ctx context.Context, collection string, hits []vectordb.Hit) ([]vectordb.Hit, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	out := make([]vectordb.Hit, 0, len(hits))
	for _, hit := range hits {
		if f.keep == nil || f.keep(hit) {
			out = append(out, hit)
		}
	}
	return out, nil
}

// TestRetrievePostFilter 覆盖后置过滤：应用、错误透传与集合消失
func TestRetrievePostFilter(t *testing.T) {
	t.Parallel()
	e, _, store := newTestEngine(t)
	ctx := context.Background()
	if err := store.CreateCollection(ctx, testCollection, testDimension); err != nil {
		t.Fatal(err)
	}
	records := []vectordb.Record{
		{ID: "doc-a", Vector: []float32{1, 0, 0}, Metadata: map[string]string{"keep": "1"}},
		{ID: "doc-b", Vector: []float32{1, 0, 0}, Metadata: map[string]string{"keep": "0"}},
	}
	if err := store.Upsert(ctx, testCollection, records); err != nil {
		t.Fatal(err)
	}
	// 后置过滤生效：只保留 keep=1 的命中
	filter := &fakeHitFilter{keep: func(hit vectordb.Hit) bool { return hit.Metadata["keep"] == "1" }}
	hits, err := e.Retrieve(ctx, testCollection, RetrieveRequest{Query: "苹果", TopK: 5, PostFilter: filter, Dimensions: testDimension})
	if err != nil {
		t.Fatal(err)
	}
	if filter.calls != 1 || len(hits) != 1 || hits[0].ID != "doc-a" {
		t.Fatalf("后置过滤结果: %+v calls=%d", hits, filter.calls)
	}
	// nil 过滤器不改变结果
	hits, err = e.Retrieve(ctx, testCollection, RetrieveRequest{Query: "苹果", TopK: 5, Dimensions: testDimension})
	if err != nil || len(hits) != 2 {
		t.Fatalf("nil 过滤器: %+v err=%v", hits, err)
	}
	// 过滤器错误原样上抛
	boom := &fakeHitFilter{err: ErrBadResponse}
	if _, err := e.Retrieve(ctx, testCollection, RetrieveRequest{Query: "苹果", TopK: 5, PostFilter: boom, Dimensions: testDimension}); !errors.Is(err, ErrBadResponse) {
		t.Fatalf("过滤器错误应透传，实际 %v", err)
	}
	// 过滤器报告集合消失：按空结果处理
	missing := &fakeHitFilter{err: vectordb.ErrNotFound}
	hits, err = e.Retrieve(ctx, testCollection, RetrieveRequest{Query: "苹果", TopK: 5, PostFilter: missing, Dimensions: testDimension})
	if err != nil || hits == nil || len(hits) != 0 {
		t.Fatalf("集合消失应按空结果处理: %+v err=%v", hits, err)
	}
	// 空命中不调用过滤器
	quiet := &fakeHitFilter{}
	hits, err = e.Retrieve(ctx, "no_such_collection", RetrieveRequest{Query: "苹果", TopK: 5, PostFilter: quiet, Dimensions: testDimension})
	if err != nil || len(hits) != 0 || quiet.calls != 0 {
		t.Fatalf("空命中不应调用过滤器: %+v calls=%d err=%v", hits, quiet.calls, err)
	}
}
