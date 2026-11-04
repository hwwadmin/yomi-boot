// memory 实现错误分支与容量边界单测。
//
// @author zwei

package vectordb

import (
	"context"
	"testing"

	"github.com/hwwadmin/yomi-boot/component/core/errors"
)

// newMemoryStoreForTest 创建测试用内存 Store
func newMemoryStoreForTest(t *testing.T, cfg Config) Store {
	t.Helper()
	cfg.Provider = ProviderMemory
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// TestMemoryStoreClosedOperations 验证关闭后所有入口返回 ErrClosed
func TestMemoryStoreClosedOperations(t *testing.T) {
	t.Parallel()
	store := newMemoryStoreForTest(t, Config{})
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("repeat Close: %v", err)
	}
	ctx := context.Background()
	ops := []struct {
		name string
		call func() error
	}{
		{"CreateCollection", func() error { return store.CreateCollection(ctx, "c", 2) }},
		{"DropCollection", func() error { return store.DropCollection(ctx, "c") }},
		{"ReplaceCollection", func() error { return store.ReplaceCollection(ctx, "c", 2, nil) }},
		{"HasCollection", func() error { _, err := store.HasCollection(ctx, "c"); return err }},
		{"EnsureCollection", func() error { return store.EnsureCollection(ctx, "c", 2) }},
		{"Upsert", func() error { return store.Upsert(ctx, "c", []Record{{ID: "a", Vector: []float32{1, 0}}}) }},
		{"Get", func() error { _, err := store.Get(ctx, "c", []string{"a"}); return err }},
		{"ListIDs", func() error { _, err := store.ListIDs(ctx, "c", map[string]string{"k": "v"}); return err }},
		{"Delete", func() error { return store.Delete(ctx, "c", []string{"a"}) }},
		{"DeleteByFilter", func() error { return store.DeleteByFilter(ctx, "c", map[string]string{"k": "v"}) }},
		{"ReplaceByFilter", func() error { return store.ReplaceByFilter(ctx, "c", map[string]string{"k": "v"}, nil) }},
		{"Search", func() error {
			_, err := store.Search(ctx, "c", SearchQuery{Vector: []float32{1, 0}, TopK: 1})
			return err
		}},
	}
	for _, op := range ops {
		if err := op.call(); !errors.Is(err, ErrClosed) {
			t.Fatalf("%s: %v", op.name, err)
		}
	}
}

// TestMemoryStoreCanceledContext 验证取消上下文按上下文错误返回
func TestMemoryStoreCanceledContext(t *testing.T) {
	t.Parallel()
	store := newMemoryStoreForTest(t, Config{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.CreateCollection(ctx, "c", 2); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
}

// TestMemoryStoreInvalidArguments 覆盖各入口的非法入参与缺失集合
func TestMemoryStoreInvalidArguments(t *testing.T) {
	t.Parallel()
	store := newMemoryStoreForTest(t, Config{})
	ctx := context.Background()
	if err := store.CreateCollection(ctx, "c", 2); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	checks := []struct {
		name string
		err  error
		want error
	}{
		{"create bad name", store.CreateCollection(ctx, "1bad", 2), ErrInvalidArgument},
		{"create bad dim", store.CreateCollection(ctx, "c2", 0), ErrInvalidArgument},
		{"create duplicate", store.CreateCollection(ctx, "c", 2), ErrAlreadyExists},
		{"replace bad dim", store.ReplaceCollection(ctx, "c", 0, nil), ErrInvalidArgument},
		{"replace bad records", store.ReplaceCollection(ctx, "c", 2, []Record{{ID: "a", Vector: []float32{1}}}), ErrInvalidArgument},
		{"has bad name", func() error { _, err := store.HasCollection(ctx, "1bad"); return err }(), ErrInvalidArgument},
		{"ensure bad dim", store.EnsureCollection(ctx, "c", 0), ErrInvalidArgument},
		{"upsert empty records", store.Upsert(ctx, "c", nil), ErrInvalidArgument},
		{"upsert bad dim", store.Upsert(ctx, "c", []Record{{ID: "a", Vector: []float32{1}}}), ErrInvalidArgument},
		{"upsert duplicate id", store.Upsert(ctx, "c", []Record{
			{ID: "a", Vector: []float32{1, 0}},
			{ID: " a ", Vector: []float32{0, 1}},
		}), ErrInvalidArgument},
		{"upsert bad metadata", store.Upsert(ctx, "c", []Record{
			{ID: "a", Vector: []float32{1, 0}, Metadata: map[string]string{`a"b`: "v"}},
		}), ErrInvalidArgument},
		{"upsert missing collection", store.Upsert(ctx, "missing", []Record{{ID: "a", Vector: []float32{1, 0}}}), ErrNotFound},
		{"get empty ids", func() error { _, err := store.Get(ctx, "c", nil); return err }(), ErrInvalidArgument},
		{"get bad name", func() error { _, err := store.Get(ctx, "1bad", []string{"a"}); return err }(), ErrInvalidArgument},
		{"get missing collection", func() error { _, err := store.Get(ctx, "missing", []string{"a"}); return err }(), ErrNotFound},
		{"list ids empty filter", func() error { _, err := store.ListIDs(ctx, "c", nil); return err }(), ErrInvalidArgument},
		{"list ids missing collection", func() error {
			_, err := store.ListIDs(ctx, "missing", map[string]string{"k": "v"})
			return err
		}(), ErrNotFound},
		{"delete empty ids", store.Delete(ctx, "c", nil), ErrInvalidArgument},
		{"delete missing collection", store.Delete(ctx, "missing", []string{"a"}), ErrNotFound},
		{"delete by filter empty", store.DeleteByFilter(ctx, "c", nil), ErrInvalidArgument},
		{"delete by filter missing collection", store.DeleteByFilter(ctx, "missing", map[string]string{"k": "v"}), ErrNotFound},
		{"replace by filter empty", store.ReplaceByFilter(ctx, "c", nil, nil), ErrInvalidArgument},
		{"replace by filter missing collection", store.ReplaceByFilter(ctx, "missing", map[string]string{"k": "v"}, nil), ErrNotFound},
		{"search missing collection", func() error {
			_, err := store.Search(ctx, "missing", SearchQuery{Vector: []float32{1, 0}, TopK: 1})
			return err
		}(), ErrNotFound},
		{"search wrong dim", func() error {
			_, err := store.Search(ctx, "c", SearchQuery{Vector: []float32{1}, TopK: 1})
			return err
		}(), ErrInvalidArgument},
		{"search zero topk", func() error {
			_, err := store.Search(ctx, "c", SearchQuery{Vector: []float32{1, 0}, TopK: 0})
			return err
		}(), ErrInvalidArgument},
		{"search too large topk", func() error {
			_, err := store.Search(ctx, "c", SearchQuery{Vector: []float32{1, 0}, TopK: MaxTopK + 1})
			return err
		}(), ErrInvalidArgument},
		{"search bad filter", func() error {
			_, err := store.Search(ctx, "c", SearchQuery{
				Vector: []float32{1, 0}, TopK: 1, Filter: map[string]string{`a"b`: "v"},
			})
			return err
		}(), ErrInvalidArgument},
	}
	for _, tc := range checks {
		if !errors.Is(tc.err, tc.want) {
			t.Fatalf("%s: got %v, want %v", tc.name, tc.err, tc.want)
		}
	}
}

// TestMemoryStoreCapacityLimit 验证总条数上限在写入与覆盖时生效
func TestMemoryStoreCapacityLimit(t *testing.T) {
	t.Parallel()
	store := newMemoryStoreForTest(t, Config{Memory: MemoryConfig{MaxRecords: 2}})
	ctx := context.Background()
	if err := store.CreateCollection(ctx, "cap", 2); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	vec := []float32{1, 0}
	if err := store.Upsert(ctx, "cap", []Record{{ID: "a", Vector: vec}, {ID: "b", Vector: vec}}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := store.Upsert(ctx, "cap", []Record{{ID: "c", Vector: vec}}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("over capacity: %v", err)
	}
	// 覆盖已有主键不增加总数
	if err := store.Upsert(ctx, "cap", []Record{{ID: "a", Vector: vec}}); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	if err := store.ReplaceCollection(ctx, "cap", 2, []Record{
		{ID: "a", Vector: vec}, {ID: "b", Vector: vec}, {ID: "c", Vector: vec},
	}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("replace over capacity: %v", err)
	}
	if err := store.ReplaceCollection(ctx, "cap", 2, []Record{{ID: "a", Vector: vec}, {ID: "b", Vector: vec}}); err != nil {
		t.Fatalf("replace at capacity: %v", err)
	}
	if err := store.CreateCollection(ctx, "cap2", 2); err != nil {
		t.Fatalf("CreateCollection cap2: %v", err)
	}
	if err := store.Upsert(ctx, "cap2", []Record{{ID: "x", Vector: vec}}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("cross collection capacity: %v", err)
	}
}

// TestMemoryStoreDimensionMismatch 验证已存在集合的维度不可变
func TestMemoryStoreDimensionMismatch(t *testing.T) {
	t.Parallel()
	store := newMemoryStoreForTest(t, Config{})
	ctx := context.Background()
	if err := store.CreateCollection(ctx, "d", 2); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	if err := store.ReplaceCollection(ctx, "d", 3, nil); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("replace dim mismatch: %v", err)
	}
	if err := store.EnsureCollection(ctx, "d", 3); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("ensure dim mismatch: %v", err)
	}
	if err := store.CreateCollection(ctx, "d", 3); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("create duplicate: %v", err)
	}
}

// TestMemoryStoreGetDedup 验证重复与带空白主键只返回一条
func TestMemoryStoreGetDedup(t *testing.T) {
	t.Parallel()
	store := newMemoryStoreForTest(t, Config{})
	ctx := context.Background()
	if err := store.CreateCollection(ctx, "g", 2); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	if err := store.Upsert(ctx, "g", []Record{{ID: "a", Vector: []float32{1, 0}, Metadata: map[string]string{"k": "v"}}}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, err := store.Get(ctx, "g", []string{" a ", "a", "missing"})
	if err != nil || len(got) != 1 || got[0].ID != "a" || got[0].Metadata["k"] != "v" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

// TestMemoryStoreBadCollectionNames 验证带集合名的入口都拒绝非法名称
func TestMemoryStoreBadCollectionNames(t *testing.T) {
	t.Parallel()
	store := newMemoryStoreForTest(t, Config{})
	ctx := context.Background()
	checks := []struct {
		name string
		err  error
	}{
		{"drop", store.DropCollection(ctx, "1bad")},
		{"replace", store.ReplaceCollection(ctx, "1bad", 2, nil)},
		{"upsert", store.Upsert(ctx, "1bad", []Record{{ID: "a", Vector: []float32{1, 0}}})},
		{"list ids", func() error { _, err := store.ListIDs(ctx, "1bad", map[string]string{"k": "v"}); return err }()},
		{"delete", store.Delete(ctx, "1bad", []string{"a"})},
		{"delete by filter", store.DeleteByFilter(ctx, "1bad", map[string]string{"k": "v"})},
		{"replace by filter", store.ReplaceByFilter(ctx, "1bad", map[string]string{"k": "v"}, nil)},
		{"search", func() error {
			_, err := store.Search(ctx, "1bad", SearchQuery{Vector: []float32{1, 0}, TopK: 1})
			return err
		}()},
	}
	for _, tc := range checks {
		if !errors.Is(tc.err, ErrInvalidArgument) {
			t.Fatalf("%s: %v", tc.name, tc.err)
		}
	}
}

// TestMemoryStoreReplaceByFilterBranches 覆盖替换写入失败与无需清理两种情况
func TestMemoryStoreReplaceByFilterBranches(t *testing.T) {
	t.Parallel()
	store := newMemoryStoreForTest(t, Config{})
	ctx := context.Background()
	if err := store.CreateCollection(ctx, "rbf", 2); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	filter := map[string]string{"doc_id": "d1"}
	// 写入记录维度不符时由 Upsert 拒绝
	if err := store.ReplaceByFilter(ctx, "rbf", filter, []Record{{ID: "x", Vector: []float32{1}}}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("bad records: %v", err)
	}
	if err := store.Upsert(ctx, "rbf", []Record{
		{ID: "x", Vector: []float32{1, 0}, Metadata: map[string]string{"doc_id": "d1"}},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	// 旧主键都在新记录中时无需清理
	if err := store.ReplaceByFilter(ctx, "rbf", filter, []Record{
		{ID: "x", Vector: []float32{0, 1}, Metadata: map[string]string{"doc_id": "d1"}},
	}); err != nil {
		t.Fatalf("ReplaceByFilter: %v", err)
	}
	ids, err := store.ListIDs(ctx, "rbf", filter)
	if err != nil || len(ids) != 1 || ids[0] != "x" {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
}

// TestMemoryStoreHybridTopKTruncate 验证融合结果按 TopK 截断
func TestMemoryStoreHybridTopKTruncate(t *testing.T) {
	t.Parallel()
	store := newMemoryStoreForTest(t, Config{})
	ctx := context.Background()
	if err := store.CreateCollection(ctx, "hybrid_topk", 2); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	vec := []float32{1, 0}
	if err := store.Upsert(ctx, "hybrid_topk", []Record{
		{ID: "a", Vector: vec, Text: "退款政策"},
		{ID: "b", Vector: vec, Text: "退款流程"},
		{ID: "c", Vector: vec, Text: "退款说明"},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	hits, err := store.Search(ctx, "hybrid_topk", SearchQuery{Vector: vec, Text: "退款", TopK: 1})
	if err != nil || len(hits) != 1 {
		t.Fatalf("hits=%+v err=%v", hits, err)
	}
}

// TestMemoryStoreHasCollection 验证缺失集合返回 false 且无错误
func TestMemoryStoreHasCollection(t *testing.T) {
	t.Parallel()
	store := newMemoryStoreForTest(t, Config{})
	ok, err := store.HasCollection(context.Background(), "missing")
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}
