// Store 契约测试。
//
// @author zwei

package vectordb

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

const (
	contractCollMut = "YomiVdbTstMut"
	contractCollNew = "YomiVdbTstNew"
	contractCollQry = "YomiVdbTstQry"
	contractDim     = 2
)

// Milvus 契约地址；默认空则跳过，运行前设置 YOMI_TEST_MILVUS_ADDRESS
var testMilvusAddress = strings.TrimSpace(os.Getenv("YOMI_TEST_MILVUS_ADDRESS"))

// 测试人员跑 Milvus 契约时填入
const (
	testMilvusDBName    = ""
	testMilvusUsername  = ""
	testMilvusPassword  = ""
	testMilvusAPIKey    = ""
	testMilvusEnableTLS = false
)

// TestStoreContractMemory 以 memory 实现跑完整门面契约
func TestStoreContractMemory(t *testing.T) {
	t.Parallel()
	store, err := New(Config{Provider: ProviderMemory})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	runStoreMutateContract(t, store, contractDim)
	runStoreQueryContract(t, store, contractDim)
}

// TestStoreContractMilvusMutate 以 milvus 跑非查询动作（DDL/DML）
func TestStoreContractMilvusMutate(t *testing.T) {
	store := newMilvusContractStore(t, contractCollMut, contractCollNew)
	runStoreMutateContract(t, store, contractDim)
}

// TestStoreContractMilvusQuery 以 milvus 跑查询类动作（Search/Get）
func TestStoreContractMilvusQuery(t *testing.T) {
	store := newMilvusContractStore(t, contractCollQry)
	runStoreQueryContract(t, store, contractDim)
}

// newMilvusContractStore 按常量建连并清理指定集合；Address 为空则跳过
func newMilvusContractStore(t *testing.T, names ...string) Store {
	t.Helper()
	addr := strings.TrimSpace(testMilvusAddress)
	if addr == "" {
		t.Skip("请填写 testMilvusAddress 等连接常量后再跑 Milvus 契约")
	}
	store, err := New(Config{
		Provider: ProviderMilvus,
		Timeout:  60 * time.Second,
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
	for _, name := range names {
		if err := store.DropCollection(ctx, name); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, name := range names {
			_ = store.DropCollection(context.Background(), name)
		}
	})
	return store
}

// contractVec 构造长度为 dim、首分量是 x 的向量
func contractVec(dim int, x float32) []float32 {
	v := make([]float32, dim)
	v[0] = x
	return v
}

// runStoreMutateContract 覆盖建删、写入、覆盖、删除；不调用 Search/Get
func runStoreMutateContract(t *testing.T, store Store, dim int) {
	t.Helper()
	ctx := context.Background()
	name := contractCollMut
	vec := func(x float32) []float32 { return contractVec(dim, x) }

	t.Run("create and has", func(t *testing.T) {
		ok, err := store.HasCollection(ctx, name)
		if err != nil || ok {
			t.Fatalf("has before create: ok=%v err=%v", ok, err)
		}
		if err := store.CreateCollection(ctx, name, dim); err != nil {
			t.Fatal(err)
		}
		ok, err = store.HasCollection(ctx, name)
		if err != nil || !ok {
			t.Fatalf("has after create: ok=%v err=%v", ok, err)
		}
	})

	t.Run("per collection dimension", func(t *testing.T) {
		if err := store.EnsureCollection(ctx, name, dim); err != nil {
			t.Fatalf("已存在且维度一致: %v", err)
		}
		if err := store.EnsureCollection(ctx, name, dim+1); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("已存在但维度不一致: %v", err)
		}
		if err := store.CreateCollection(ctx, contractCollNew, dim+1); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.DropCollection(context.Background(), contractCollNew) })
		if err := store.Upsert(ctx, contractCollNew, []Record{{ID: "w1", Vector: contractVec(dim+1, 1)}}); err != nil {
			t.Fatalf("不同维度集合写入: %v", err)
		}
		if err := store.Upsert(ctx, contractCollNew, []Record{{ID: "w2", Vector: contractVec(dim, 1)}}); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("集合维度不符应拒绝: %v", err)
		}
	})

	t.Run("create duplicate", func(t *testing.T) {
		err := store.CreateCollection(ctx, name, dim)
		if !errors.Is(err, ErrAlreadyExists) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("upsert", func(t *testing.T) {
		if err := store.Upsert(ctx, name, []Record{
			{ID: "a", Vector: vec(1), Metadata: map[string]string{"doc_id": "d1"}},
			{ID: "b", Vector: vec(0), Metadata: map[string]string{"doc_id": "d2"}},
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("upsert overwrite", func(t *testing.T) {
		if err := store.Upsert(ctx, name, []Record{
			{ID: "a", Vector: vec(1), Metadata: map[string]string{"doc_id": "d1v2"}},
		}); err != nil {
			t.Fatal(err)
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
		if err := store.DeleteByFilter(ctx, name, nil); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("empty filter: %v", err)
		}
	})

	t.Run("replace snapshot", func(t *testing.T) {
		if err := store.ReplaceCollection(ctx, name, dim, []Record{
			{ID: "z", Vector: vec(1), Metadata: map[string]string{"k": "v"}},
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("replace empty keeps collection", func(t *testing.T) {
		if err := store.ReplaceCollection(ctx, name, dim, nil); err != nil {
			t.Fatal(err)
		}
		ok, err := store.HasCollection(ctx, name)
		if err != nil || !ok {
			t.Fatalf("has after empty replace: ok=%v err=%v", ok, err)
		}
	})

	t.Run("ensure collection", func(t *testing.T) {
		if err := store.EnsureCollection(ctx, contractCollNew, dim); err != nil {
			t.Fatalf("已存在时 ensure: %v", err)
		}
		if err := store.DropCollection(ctx, contractCollNew); err != nil {
			t.Fatal(err)
		}
		if err := store.EnsureCollection(ctx, contractCollNew, dim); err != nil {
			t.Fatalf("不存在时 ensure: %v", err)
		}
		ok, err := store.HasCollection(ctx, contractCollNew)
		if err != nil || !ok {
			t.Fatalf("ensure 后 has: ok=%v err=%v", ok, err)
		}
		if err := store.EnsureCollection(ctx, "bad-name", dim); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("bad name: %v", err)
		}
	})

	t.Run("replace missing creates", func(t *testing.T) {
		if err := store.ReplaceCollection(ctx, contractCollNew, dim, []Record{
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
	})

	t.Run("missing collection dml", func(t *testing.T) {
		err := store.Upsert(ctx, "idx_none", []Record{{ID: "x", Vector: vec(1)}})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("upsert missing: %v", err)
		}
	})

	t.Run("invalid args", func(t *testing.T) {
		if err := store.CreateCollection(ctx, "bad-name", dim); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("bad name: %v", err)
		}
		if err := store.CreateCollection(ctx, name, dim); err != nil {
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
		if err := store.Upsert(ctx, name, []Record{{
			ID: "bad-meta-quote", Vector: vec(1), Metadata: map[string]string{`a"b`: "x"},
		}}); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("bad metadata key: %v", err)
		}
		if err := store.Upsert(ctx, name, []Record{{
			ID: "bad-meta-dup", Vector: vec(1), Metadata: map[string]string{"k": "1", " k ": "2"},
		}}); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("duplicate normalized metadata key: %v", err)
		}
	})
}

// runStoreQueryContract 覆盖 Search/Get；自行建集写入种子数据
func runStoreQueryContract(t *testing.T, store Store, dim int) {
	t.Helper()
	ctx := context.Background()
	name := contractCollQry
	vec := func(x float32) []float32 { return contractVec(dim, x) }

	t.Run("empty search", func(t *testing.T) {
		if err := store.CreateCollection(ctx, name, dim); err != nil {
			t.Fatal(err)
		}
		hits, err := store.Search(ctx, name, SearchQuery{Vector: vec(1), TopK: 3})
		if err != nil || len(hits) != 0 {
			t.Fatalf("empty search: hits=%v err=%v", hits, err)
		}
	})

	t.Run("search cosine", func(t *testing.T) {
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

	t.Run("get after overwrite", func(t *testing.T) {
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

	t.Run("list ids by filter", func(t *testing.T) {
		ids, err := store.ListIDs(ctx, name, map[string]string{"doc_id": "d2"})
		if err != nil || len(ids) != 1 || ids[0] != "b" {
			t.Fatalf("list ids: %+v err=%v", ids, err)
		}
		ids, err = store.ListIDs(ctx, name, map[string]string{"doc_id": "nope"})
		if err != nil || len(ids) != 0 {
			t.Fatalf("list ids miss: %+v err=%v", ids, err)
		}
		if _, err := store.ListIDs(ctx, name, nil); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("list ids empty filter: %v", err)
		}
	})

	t.Run("get after delete", func(t *testing.T) {
		if err := store.Delete(ctx, name, []string{"b"}); err != nil {
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
	})

	t.Run("search after replace", func(t *testing.T) {
		if err := store.ReplaceCollection(ctx, name, dim, []Record{
			{ID: "z", Vector: vec(1), Metadata: map[string]string{"k": "v"}},
		}); err != nil {
			t.Fatal(err)
		}
		got, err := store.Get(ctx, name, []string{"a", "z"})
		if err != nil || len(got) != 1 || got[0].ID != "z" {
			t.Fatalf("replace leftover: %+v err=%v", got, err)
		}
		if err := store.ReplaceCollection(ctx, name, dim, nil); err != nil {
			t.Fatal(err)
		}
		hits, err := store.Search(ctx, name, SearchQuery{Vector: vec(1), TopK: 3})
		if err != nil || len(hits) != 0 {
			t.Fatalf("empty replace search: %+v err=%v", hits, err)
		}
	})

	t.Run("hybrid search", func(t *testing.T) {
		if err := store.Upsert(ctx, name, []Record{
			{ID: "hx", Vector: vec(0), Text: "退款政策与流程", Metadata: map[string]string{"doc_id": "dhx"}},
			{ID: "hy", Vector: vec(0), Text: "配送范围与时效", Metadata: map[string]string{"doc_id": "dhy"}},
			{ID: "hz", Vector: vec(0)},
		}); err != nil {
			t.Fatal(err)
		}
		// 向量完全相同（正交于查询向量），排名差异只可能来自 BM25 词法命中
		hits, err := store.Search(ctx, name, SearchQuery{Vector: vec(1), Text: "如何退款", TopK: 3})
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) == 0 || hits[0].ID != "hx" {
			t.Fatalf("词法命中应排第一: %+v", hits)
		}
		if hits[0].Score <= 0 || hits[0].Score > 1.0 {
			t.Fatalf("RRF 融合分超界: %+v", hits)
		}
	})

	t.Run("replace by filter", func(t *testing.T) {
		if err := store.ReplaceCollection(ctx, name, dim, []Record{
			{ID: "d1-0", Vector: vec(1), Metadata: map[string]string{"doc_id": "d1"}},
			{ID: "d1-1", Vector: vec(1), Metadata: map[string]string{"doc_id": "d1"}},
			{ID: "d2-0", Vector: vec(1), Metadata: map[string]string{"doc_id": "d2"}},
		}); err != nil {
			t.Fatal(err)
		}
		// 覆盖 d1：写入新块 d1-0，清理 d1-1，d2 不受影响
		if err := store.ReplaceByFilter(ctx, name, map[string]string{"doc_id": "d1"}, []Record{
			{ID: "d1-0", Vector: vec(0.5), Metadata: map[string]string{"doc_id": "d1"}},
		}); err != nil {
			t.Fatal(err)
		}
		ids, err := store.ListIDs(ctx, name, map[string]string{"doc_id": "d1"})
		if err != nil || len(ids) != 1 || ids[0] != "d1-0" {
			t.Fatalf("d1 替换结果: %+v err=%v", ids, err)
		}
		ids, err = store.ListIDs(ctx, name, map[string]string{"doc_id": "d2"})
		if err != nil || len(ids) != 1 {
			t.Fatalf("d2 应不受影响: %+v err=%v", ids, err)
		}
		// 空 records 只清理
		if err := store.ReplaceByFilter(ctx, name, map[string]string{"doc_id": "d2"}, nil); err != nil {
			t.Fatal(err)
		}
		ids, err = store.ListIDs(ctx, name, map[string]string{"doc_id": "d2"})
		if err != nil || len(ids) != 0 {
			t.Fatalf("空 records 清理: %+v err=%v", ids, err)
		}
		// 参数校验与集合缺失
		if err := store.ReplaceByFilter(ctx, name, nil, nil); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("empty filter: %v", err)
		}
		if err := store.ReplaceByFilter(ctx, "idx_none", map[string]string{"k": "v"}, nil); !errors.Is(err, ErrNotFound) {
			t.Fatalf("missing collection: %v", err)
		}
	})

	t.Run("search dropped", func(t *testing.T) {
		if err := store.DropCollection(ctx, name); err != nil {
			t.Fatal(err)
		}
		_, err := store.Search(ctx, name, SearchQuery{Vector: vec(1), TopK: 1})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("search dropped: %v", err)
		}
		_, err = store.Get(ctx, name, []string{"z"})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("get dropped: %v", err)
		}
		_, err = store.ListIDs(ctx, name, map[string]string{"k": "v"})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("list ids dropped: %v", err)
		}
	})

	t.Run("invalid search", func(t *testing.T) {
		if err := store.CreateCollection(ctx, name, dim); err != nil {
			t.Fatal(err)
		}
		_, err := store.Search(ctx, name, SearchQuery{Vector: vec(1), TopK: 0})
		if !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("topk: %v", err)
		}
	})
}
