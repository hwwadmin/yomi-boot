// Package vectordb Store 契约测试。
//
// @author zwei
package vectordb

import (
	"context"
	"strings"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"
)

const (
	contractCollMain = "YomiVdbTstMain"
	contractCollNew  = "YomiVdbTstNew"
	contractDim      = 2
)

// 测试人员跑 TestStoreContractMilvus 时填入；Address 为空则跳过
const (
	testMilvusAddress   = ""
	testMilvusDBName    = ""
	testMilvusUsername  = ""
	testMilvusPassword  = ""
	testMilvusAPIKey    = ""
	testMilvusEnableTLS = false
)

// TestStoreContractMemory 以 memory 实现跑完整门面契约
func TestStoreContractMemory(t *testing.T) {
	t.Parallel()
	store, err := New(Config{Provider: ProviderMemory, Dimension: contractDim})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	runStoreContract(t, store, contractDim)
}

// TestStoreContractMilvus 以 milvus 实现跑完整门面契约；连接常量未填则跳过
func TestStoreContractMilvus(t *testing.T) {
	addr := strings.TrimSpace(testMilvusAddress)
	if addr == "" {
		t.Skip("请填写 testMilvusAddress 等连接常量后再跑 Milvus 契约")
	}
	store, err := New(Config{
		Provider:  ProviderMilvus,
		Dimension: contractDim,
		Timeout:   60 * time.Second,
		Milvus: MilvusConfig{
			Address:   addr,
			DBName:    strings.TrimSpace(testMilvusDBName),
			Username:  strings.TrimSpace(testMilvusUsername),
			Password:  testMilvusPassword,
			APIKey:    testMilvusAPIKey,
			EnableTLS: testMilvusEnableTLS,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	for _, name := range []string{contractCollMain, contractCollNew} {
		if err := store.DropCollection(ctx, name); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_ = store.DropCollection(context.Background(), contractCollMain)
		_ = store.DropCollection(context.Background(), contractCollNew)
	})
	runStoreContract(t, store, contractDim)
}

// runStoreContract 对任意 Store 跑契约用例
func runStoreContract(t *testing.T, store Store, dim int) {
	t.Helper()
	ctx := context.Background()
	name := contractCollMain
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
		if err := store.ReplaceCollection(ctx, contractCollNew, []Record{
			{ID: "n1", Vector: vec(1)},
		}); err != nil {
			t.Fatal(err)
		}
		ok, err := store.HasCollection(ctx, contractCollNew)
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
}
