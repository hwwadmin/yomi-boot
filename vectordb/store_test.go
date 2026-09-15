// Package vectordb Store 契约测试（默认 memory）。
//
// @author zwei
package vectordb

import (
	"context"
	"testing"

	"gitee.com/hq_project/yomi-boot/errors"
)

// TestStoreContractMemory 以 memory 实现跑完整门面契约
func TestStoreContractMemory(t *testing.T) {
	t.Parallel()
	store, err := New(Config{Provider: ProviderMemory, Dimension: 2})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	runStoreContract(t, store, 2)
}

// TestStoreContractMemoryL2 验证 L2 下更近的排前面
func TestStoreContractMemoryL2(t *testing.T) {
	t.Parallel()
	store, err := New(Config{Provider: ProviderMemory, Dimension: 2, Metric: MetricL2})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.CreateCollection(ctx, "idx_l2"); err != nil {
		t.Fatal(err)
	}
	if err := store.Upsert(ctx, "idx_l2", []Record{
		{ID: "far", Vector: []float32{3, 0}},
		{ID: "near", Vector: []float32{1, 0}},
	}); err != nil {
		t.Fatal(err)
	}
	hits, err := store.Search(ctx, "idx_l2", SearchQuery{Vector: []float32{0, 0}, TopK: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 || hits[0].ID != "near" {
		t.Fatalf("L2 ranking got %+v", hits)
	}
	if hits[0].Score <= hits[1].Score {
		t.Fatalf("L2 score should be higher for nearer: %+v", hits)
	}
}

// TestStoreContractMemoryIP 验证 IP 下内积更大的排前面
func TestStoreContractMemoryIP(t *testing.T) {
	t.Parallel()
	store, err := New(Config{Provider: ProviderMemory, Dimension: 2, Metric: MetricIP})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.CreateCollection(ctx, "idx_ip"); err != nil {
		t.Fatal(err)
	}
	if err := store.Upsert(ctx, "idx_ip", []Record{
		{ID: "low", Vector: []float32{1, 0}},
		{ID: "high", Vector: []float32{5, 0}},
	}); err != nil {
		t.Fatal(err)
	}
	hits, err := store.Search(ctx, "idx_ip", SearchQuery{Vector: []float32{1, 0}, TopK: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 || hits[0].ID != "high" {
		t.Fatalf("IP ranking got %+v", hits)
	}
}

// runStoreContract 对任意 Store 跑契约用例
func runStoreContract(t *testing.T, store Store, dim int) {
	t.Helper()
	ctx := context.Background()
	name := "idx_main"
	vec := func(x float32) []float32 {
		v := make([]float32, dim)
		v[0] = x
		return v
	}

	t.Run("create and has", func(t *testing.T) {
		ok, err := store.HasCollection(ctx, name)
		if err != nil || ok {
			t.Fatalf("has before create: ok=%v err=%v", ok, err)
		}
		if err := store.CreateCollection(ctx, name); err != nil {
			t.Fatal(err)
		}
		ok, err = store.HasCollection(ctx, name)
		if err != nil || !ok {
			t.Fatalf("has after create: ok=%v err=%v", ok, err)
		}
		hits, err := store.Search(ctx, name, SearchQuery{Vector: vec(1), TopK: 3})
		if err != nil || len(hits) != 0 {
			t.Fatalf("empty search: hits=%v err=%v", hits, err)
		}
	})

	t.Run("create duplicate", func(t *testing.T) {
		err := store.CreateCollection(ctx, name)
		if !errors.Is(err, ErrAlreadyExists) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("upsert search cosine", func(t *testing.T) {
		if err := store.Upsert(ctx, name, []Record{
			{ID: "a", Vector: vec(1), Metadata: map[string]string{"doc_id": "d1"}},
			{ID: "b", Vector: vec(0), Metadata: map[string]string{"doc_id": "d2"}},
		}); err != nil {
			t.Fatal(err)
		}
		hits, err := store.Search(ctx, name, SearchQuery{Vector: vec(1), TopK: 1})
		if err != nil || len(hits) != 1 || hits[0].ID != "a" {
			t.Fatalf("nearest: %+v err=%v", hits, err)
		}
		if hits[0].Metadata["doc_id"] != "d1" {
			t.Fatalf("metadata: %+v", hits[0].Metadata)
		}
	})

	t.Run("upsert overwrite", func(t *testing.T) {
		if err := store.Upsert(ctx, name, []Record{
			{ID: "a", Vector: vec(1), Metadata: map[string]string{"doc_id": "d1v2"}},
		}); err != nil {
			t.Fatal(err)
		}
		got, err := store.Get(ctx, name, []string{"a", "missing"})
		if err != nil || len(got) != 1 || got[0].Metadata["doc_id"] != "d1v2" {
			t.Fatalf("get: %+v err=%v", got, err)
		}
	})

	t.Run("filter search", func(t *testing.T) {
		hits, err := store.Search(ctx, name, SearchQuery{
			Vector: vec(1),
			TopK:   10,
			Filter: map[string]string{"doc_id": "d2"},
		})
		if err != nil || len(hits) != 1 || hits[0].ID != "b" {
			t.Fatalf("filter: %+v err=%v", hits, err)
		}
		hits, err = store.Search(ctx, name, SearchQuery{
			Vector: vec(1),
			TopK:   10,
			Filter: map[string]string{"doc_id": "nope"},
		})
		if err != nil || len(hits) != 0 {
			t.Fatalf("miss filter: %+v err=%v", hits, err)
		}
	})

	t.Run("delete by id and filter", func(t *testing.T) {
		if err := store.Delete(ctx, name, []string{"missing", "b"}); err != nil {
			t.Fatal(err)
		}
		if err := store.Upsert(ctx, name, []Record{
			{ID: "c", Vector: vec(1), Metadata: map[string]string{"doc_id": "dx"}},
			{ID: "d", Vector: vec(1), Metadata: map[string]string{"doc_id": "dx"}},
		}); err != nil {
			t.Fatal(err)
		}
		if err := store.DeleteByFilter(ctx, name, map[string]string{"doc_id": "dx"}); err != nil {
			t.Fatal(err)
		}
		got, err := store.Get(ctx, name, []string{"c", "d", "a"})
		if err != nil || len(got) != 1 || got[0].ID != "a" {
			t.Fatalf("after filter delete: %+v err=%v", got, err)
		}
		if err := store.DeleteByFilter(ctx, name, nil); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("empty filter: %v", err)
		}
	})

	t.Run("replace snapshot", func(t *testing.T) {
		if err := store.ReplaceCollection(ctx, name, []Record{
			{ID: "z", Vector: vec(1), Metadata: map[string]string{"k": "v"}},
		}); err != nil {
			t.Fatal(err)
		}
		got, err := store.Get(ctx, name, []string{"a", "z"})
		if err != nil || len(got) != 1 || got[0].ID != "z" {
			t.Fatalf("replace leftover: %+v err=%v", got, err)
		}
	})

	t.Run("replace empty keeps collection", func(t *testing.T) {
		if err := store.ReplaceCollection(ctx, name, nil); err != nil {
			t.Fatal(err)
		}
		ok, err := store.HasCollection(ctx, name)
		if err != nil || !ok {
			t.Fatalf("has after empty replace: ok=%v err=%v", ok, err)
		}
		hits, err := store.Search(ctx, name, SearchQuery{Vector: vec(1), TopK: 3})
		if err != nil || len(hits) != 0 {
			t.Fatalf("empty replace search: %+v err=%v", hits, err)
		}
	})

	t.Run("replace missing creates", func(t *testing.T) {
		if err := store.ReplaceCollection(ctx, "idx_new", []Record{
			{ID: "n1", Vector: vec(1)},
		}); err != nil {
			t.Fatal(err)
		}
		ok, err := store.HasCollection(ctx, "idx_new")
		if err != nil || !ok {
			t.Fatalf("replace create: ok=%v err=%v", ok, err)
		}
	})

	t.Run("drop", func(t *testing.T) {
		if err := store.DropCollection(ctx, name); err != nil {
			t.Fatal(err)
		}
		if err := store.DropCollection(ctx, name); err != nil {
			t.Fatal(err)
		}
		ok, err := store.HasCollection(ctx, name)
		if err != nil || ok {
			t.Fatalf("has after drop: ok=%v err=%v", ok, err)
		}
		_, err = store.Search(ctx, name, SearchQuery{Vector: vec(1), TopK: 1})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("search dropped: %v", err)
		}
	})

	t.Run("missing collection dml", func(t *testing.T) {
		err := store.Upsert(ctx, "idx_none", []Record{{ID: "x", Vector: vec(1)}})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("upsert missing: %v", err)
		}
	})

	t.Run("invalid args", func(t *testing.T) {
		if err := store.CreateCollection(ctx, "bad-name"); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("bad name: %v", err)
		}
		if err := store.CreateCollection(ctx, name); err != nil {
			t.Fatal(err)
		}
		if err := store.Upsert(ctx, name, []Record{{ID: "x", Vector: []float32{1}}}); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("bad dim: %v", err)
		}
		if err := store.Upsert(ctx, name, []Record{
			{ID: "dup", Vector: vec(1)},
			{ID: "dup", Vector: vec(1)},
		}); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("dup id: %v", err)
		}
		_, err := store.Search(ctx, name, SearchQuery{Vector: vec(1), TopK: 0})
		if !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("topk: %v", err)
		}
	})

	t.Run("max records", func(t *testing.T) {
		small, err := New(Config{Provider: ProviderMemory, Dimension: dim, Memory: MemoryConfig{MaxRecords: 1}})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = small.Close() }()
		if err := small.CreateCollection(ctx, "idx_cap"); err != nil {
			t.Fatal(err)
		}
		if err := small.Upsert(ctx, "idx_cap", []Record{{ID: "1", Vector: vec(1)}}); err != nil {
			t.Fatal(err)
		}
		if err := small.Upsert(ctx, "idx_cap", []Record{{ID: "2", Vector: vec(1)}}); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("cap: %v", err)
		}
	})
}

// TestStoreClosed 验证 Close 之后返回 ErrClosed
func TestStoreClosed(t *testing.T) {
	t.Parallel()
	store, err := New(Config{Provider: ProviderMemory, Dimension: 2})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateCollection(context.Background(), "idx_x"); !errors.Is(err, ErrClosed) {
		t.Fatalf("got %v", err)
	}
	ok, err := store.HasCollection(context.Background(), "idx_x")
	if ok || !errors.Is(err, ErrClosed) {
		t.Fatalf("has closed: ok=%v err=%v", ok, err)
	}
}

// TestStoreCanceledContext 验证已取消的 ctx 不会进入检索
func TestStoreCanceledContext(t *testing.T) {
	t.Parallel()
	store, err := New(Config{Provider: ProviderMemory, Dimension: 2})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.CreateCollection(ctx, "idx_ctx"); err != nil {
		t.Fatal(err)
	}
	if err := store.Upsert(ctx, "idx_ctx", []Record{{ID: "a", Vector: []float32{1, 0}}}); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = store.Search(canceled, "idx_ctx", SearchQuery{Vector: []float32{1, 0}, TopK: 1})
	if err == nil {
		t.Fatal("expected canceled ctx error")
	}
}
