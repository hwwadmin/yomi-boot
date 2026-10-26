// 相似度打分与度量映射单测。
//
// @author zwei

package vectordb

import (
	"context"
	"math"
	"testing"
)

// TestSimilarityScoreMetrics 覆盖三种度量、空度量回退与边界向量
func TestSimilarityScoreMetrics(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		metric Metric
		a, b   []float32
		want   float32
	}{
		{"cosine identical", MetricCosine, []float32{1, 0}, []float32{1, 0}, 1},
		{"cosine empty metric", "", []float32{1, 0}, []float32{1, 0}, 1},
		{"cosine zero vector", MetricCosine, []float32{0, 0}, []float32{1, 0}, 0},
		{"cosine shorter second", MetricCosine, []float32{1, 0, 0}, []float32{1, 0}, 1},
		{"l2 distance", MetricL2, []float32{0, 0}, []float32{3, 4}, 1.0 / 6.0},
		{"ip dot", MetricIP, []float32{1, 2}, []float32{3, 4}, 11},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := similarityScore(tc.metric, tc.a, tc.b)
			if math.Abs(float64(got-tc.want)) > 1e-6 {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestScoreHelpers 覆盖点积、欧氏距离与原始分映射
func TestScoreHelpers(t *testing.T) {
	t.Parallel()
	if got := dot([]float32{1, 2, 3}, []float32{4, 5}); got != 14 {
		t.Fatalf("dot=%v", got)
	}
	if got := l2Distance([]float32{0, 0, 0}, []float32{3, 4}); got != 5 {
		t.Fatalf("l2=%v", got)
	}
	if got := l2ToScore(0); got != 1 {
		t.Fatalf("l2ToScore(0)=%v", got)
	}
	if got := l2ToScore(1); got != 0.5 {
		t.Fatalf("l2ToScore(1)=%v", got)
	}
	if got := engineScoreToHit(MetricL2, 5); math.Abs(float64(got-1.0/6.0)) > 1e-6 {
		t.Fatalf("l2 hit score=%v", got)
	}
	if got := engineScoreToHit(MetricCosine, 0.7); got != 0.7 {
		t.Fatalf("cosine hit score=%v", got)
	}
}

// TestMemoryStoreMetricRanking 验证 memory 实现按配置度量排序
func TestMemoryStoreMetricRanking(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		metric Metric
		query  []float32
		rows   map[string][]float32
		want   []string
	}{
		{
			name:   "cosine",
			metric: MetricCosine,
			query:  []float32{1, 0},
			rows:   map[string][]float32{"a": {2, 0}, "b": {0, 1}},
			want:   []string{"a", "b"},
		},
		{
			name:   "l2",
			metric: MetricL2,
			query:  []float32{0, 0},
			rows:   map[string][]float32{"a": {3, 4}, "b": {1, 0}},
			want:   []string{"b", "a"},
		},
		{
			name:   "ip",
			metric: MetricIP,
			query:  []float32{1, 0},
			rows:   map[string][]float32{"a": {2, 0}, "b": {0, 1}},
			want:   []string{"a", "b"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, err := New(Config{Provider: ProviderMemory, Metric: tc.metric})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			t.Cleanup(func() { _ = store.Close() })
			ctx := context.Background()
			if err := store.CreateCollection(ctx, "metric_coll", 2); err != nil {
				t.Fatalf("CreateCollection: %v", err)
			}
			records := make([]Record, 0, len(tc.rows))
			for id, vec := range tc.rows {
				records = append(records, Record{ID: id, Vector: vec})
			}
			if err := store.Upsert(ctx, "metric_coll", records); err != nil {
				t.Fatalf("Upsert: %v", err)
			}
			hits, err := store.Search(ctx, "metric_coll", SearchQuery{Vector: tc.query, TopK: len(tc.want)})
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			if len(hits) != len(tc.want) {
				t.Fatalf("hits=%+v", hits)
			}
			for i, id := range tc.want {
				if hits[i].ID != id {
					t.Fatalf("rank[%d]=%q, want %q: %+v", i, hits[i].ID, id, hits)
				}
			}
		})
	}
}
