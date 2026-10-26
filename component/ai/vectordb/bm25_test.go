// BM25 词法评分、RRF 融合与混合检索单测。
//
// @author zwei

package vectordb

import (
	"context"
	"math"
	"reflect"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// TestTokenizeText 覆盖 ASCII 词、CJK 单字与二元词组、分隔符
func TestTokenizeText(t *testing.T) {
	t.Parallel()
	got := tokenizeText("Hello, World! 你好世界 foo123")
	want := []string{"hello", "world", "你", "好", "世", "界", "你好", "好世", "世界", "foo123"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("切词 = %q，期望 %q", got, want)
	}
	if tokenizeText("  ，。！  ") != nil {
		t.Fatalf("纯分隔符应切出空: %q", tokenizeText("  ，。！  "))
	}
}

// TestBM25ScorerLexicalHit 覆盖完整含词得分高于字符重合、零重合得 0 分、词频提升得分
func TestBM25ScorerLexicalHit(t *testing.T) {
	t.Parallel()
	scorer := newBM25Scorer([]string{
		"苹果是一种水果",
		"香蕉也是水果",
		"配送范围",
		"",
	})
	query := tokenizeText("苹果")
	full := scorer.score(0, query)
	partial := scorer.score(1, query)
	if full <= 0 {
		t.Fatalf("完整含词文档应得正分: %v", full)
	}
	// "水果"只与"苹果"共享单字"果"，得分应低于完整含二元词"苹果"的文档
	if partial >= full {
		t.Fatalf("字符重合得分 %v 不应高于完整含词 %v", partial, full)
	}
	if got := scorer.score(2, query); got != 0 {
		t.Fatalf("零重合文档应得 0 分: %v", got)
	}
	if got := scorer.score(3, query); got != 0 {
		t.Fatalf("空文档应得 0 分: %v", got)
	}
	// 多次出现的词得分更高
	scorer2 := newBM25Scorer([]string{"苹果 苹果", "苹果"})
	q := tokenizeText("苹果")
	if scorer2.score(0, q) <= scorer2.score(1, q) {
		t.Fatalf("词频应提升得分: %v vs %v", scorer2.score(0, q), scorer2.score(1, q))
	}
}

// TestRRFFuse 覆盖 RRF 融合公式与同分按 ID 升序
func TestRRFFuse(t *testing.T) {
	t.Parallel()
	dense := []rankedID{{id: "a", score: 0.9}, {id: "b", score: 0.8}}
	lexical := []rankedID{{id: "a", score: 5}, {id: "c", score: 1}}
	got := rrfFuse([][]rankedID{dense, lexical})
	if len(got) != 3 {
		t.Fatalf("融合条目 = %d，期望 3", len(got))
	}
	if got[0].id != "a" {
		t.Fatalf("a 应排第一: %+v", got)
	}
	// a 两路都第 1：1/61 + 1/61
	wantA := 2.0 / float64(rrfK+1)
	if math.Abs(got[0].score-wantA) > 1e-9 {
		t.Fatalf("a 融合分 = %v，期望 %v", got[0].score, wantA)
	}
	// b 与 c 都是单路第 2，同分按 ID 升序
	if got[1].id != "b" || got[2].id != "c" {
		t.Fatalf("同分应按 ID 升序: %+v", got)
	}
	if got[1].score != got[2].score {
		t.Fatalf("b 与 c 融合分应相等: %+v", got)
	}
}

// TestMemorySearchHybrid 覆盖 memory 混合检索：词法命中提升排名、纯向量路径不变
func TestMemorySearchHybrid(t *testing.T) {
	t.Parallel()
	store, err := New(Config{Provider: ProviderMemory})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.CreateCollection(ctx, "hybrid_t", 2); err != nil {
		t.Fatal(err)
	}
	// a/b/c 向量相同（纯向量下并列），只有 a 的原文含查询词
	vec := []float32{1, 0}
	if err := store.Upsert(ctx, "hybrid_t", []Record{
		{ID: "a", Vector: vec, Text: "退款政策说明"},
		{ID: "b", Vector: vec, Text: "配送范围说明"},
		{ID: "c", Vector: vec},
	}); err != nil {
		t.Fatal(err)
	}
	hits, err := store.Search(ctx, "hybrid_t", SearchQuery{Vector: vec, TopK: 3})
	if err != nil {
		t.Fatal(err)
	}
	// 纯向量：同分按 ID 升序
	if hits[0].ID != "a" || hits[1].ID != "b" || hits[2].ID != "c" {
		t.Fatalf("纯向量排序: %+v", hits)
	}
	hybrid, err := store.Search(ctx, "hybrid_t", SearchQuery{Vector: vec, Text: "如何退款", TopK: 3})
	if err != nil {
		t.Fatal(err)
	}
	if hybrid[0].ID != "a" {
		t.Fatalf("词法命中应排第一: %+v", hybrid)
	}
	// 混合分是 RRF 融合分，量级远小于 1
	if hybrid[0].Score <= 0 || hybrid[0].Score > 2.0/61+1e-6 {
		t.Fatalf("RRF 融合分超界: %+v", hybrid)
	}
}

// TestMemorySearchHybridFilterAndTopK 覆盖混合检索下 Filter 对两路生效与 TopK 截断
func TestMemorySearchHybridFilterAndTopK(t *testing.T) {
	t.Parallel()
	store, err := New(Config{Provider: ProviderMemory})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.CreateCollection(ctx, "hybrid_f", 2); err != nil {
		t.Fatal(err)
	}
	vec := []float32{1, 0}
	if err := store.Upsert(ctx, "hybrid_f", []Record{
		{ID: "a", Vector: vec, Text: "退款政策", Metadata: map[string]string{"cat": "x"}},
		{ID: "b", Vector: vec, Text: "退款政策", Metadata: map[string]string{"cat": "y"}},
	}); err != nil {
		t.Fatal(err)
	}
	hits, err := store.Search(ctx, "hybrid_f", SearchQuery{
		Vector: vec,
		Text:   "退款",
		TopK:   10,
		Filter: map[string]string{"cat": "y"},
	})
	if err != nil || len(hits) != 1 || hits[0].ID != "b" {
		t.Fatalf("Filter 应对混合检索生效: %+v err=%v", hits, err)
	}
	hits, err = store.Search(ctx, "hybrid_f", SearchQuery{Vector: vec, Text: "退款", TopK: 1})
	if err != nil || len(hits) != 1 {
		t.Fatalf("TopK 截断: %+v err=%v", hits, err)
	}
}

// TestSearchTextTooLong 覆盖 Text 长度上限校验
func TestSearchTextTooLong(t *testing.T) {
	t.Parallel()
	store, err := New(Config{Provider: ProviderMemory})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.CreateCollection(ctx, "text_limit", 2); err != nil {
		t.Fatal(err)
	}
	long := make([]byte, maxTextLen+1)
	for i := range long {
		long[i] = 'a'
	}
	if err := store.Upsert(ctx, "text_limit", []Record{{
		ID: "a", Vector: []float32{1, 0}, Text: string(long),
	}}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("写入超长 Text: %v", err)
	}
	if _, err := store.Search(ctx, "text_limit", SearchQuery{
		Vector: []float32{1, 0}, Text: string(long), TopK: 1,
	}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("检索超长 Text: %v", err)
	}
}
