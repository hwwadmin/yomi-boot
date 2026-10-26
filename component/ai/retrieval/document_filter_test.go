// 内置文档版本过滤器测试。
//
// @author zwei

package retrieval

import (
	"context"
	"strconv"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/ai/vectordb"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

const (
	docFilterCollection = "filter_test"
	filterDocKey        = "f_doc"
	filterIndexKey      = "f_idx"
	filterCountKey      = "f_count"
	filterVersionKey    = "f_version"
	filterContentKey    = "f_content"
)

// testDocumentFilterConfig 构造测试用过滤器配置；文档 ID 超过 5 字节不参与状态查询
func testDocumentFilterConfig() DocumentFilterConfig {
	return DocumentFilterConfig{
		DocumentKey: filterDocKey,
		IndexKey:    filterIndexKey,
		CountKey:    filterCountKey,
		VersionKey:  filterVersionKey,
		ContentKey:  filterContentKey,
		FirstChunkID: func(docID string) string {
			if len(docID) > 5 {
				return ""
			}
			return docID + "#0"
		},
	}
}

// filterHit 构造一条命中
func filterHit(docID string, index int, version, content string) vectordb.Hit {
	md := map[string]string{
		filterDocKey:     docID,
		filterIndexKey:   strconv.Itoa(index),
		filterVersionKey: version,
		filterContentKey: content,
	}
	return vectordb.Hit{ID: docID + "#" + strconv.Itoa(index), Metadata: md}
}

// getFailStore 让 Get 返回指定错误，其余透传
type getFailStore struct {
	vectordb.Store
	err error
}

// Get 模拟辅助状态查询失败
func (s getFailStore) Get(ctx context.Context, collection string, ids []string) ([]vectordb.Record, error) {
	return nil, s.err
}

// countGetStore 记录 Get 调用次数
type countGetStore struct {
	vectordb.Store
	gets int
}

// Get 记录后透传
func (s *countGetStore) Get(ctx context.Context, collection string, ids []string) ([]vectordb.Record, error) {
	s.gets++
	return s.Store.Get(ctx, collection, ids)
}

// newFilterTestStore 创建含两篇文档状态的内存向量库
func newFilterTestStore(t *testing.T) vectordb.Store {
	t.Helper()
	store, err := vectordb.New(vectordb.Config{Provider: vectordb.ProviderMemory})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.CreateCollection(ctx, docFilterCollection, testDimension); err != nil {
		t.Fatal(err)
	}
	records := []vectordb.Record{
		{ID: "doc-a#0", Vector: []float32{1, 0, 0}, Metadata: map[string]string{
			filterDocKey: "doc-a", filterCountKey: "2", filterVersionKey: "v2", filterContentKey: "新内容",
		}},
		{ID: "doc-b#0", Vector: []float32{1, 0, 0}, Metadata: map[string]string{
			filterDocKey: "doc-b", filterCountKey: "1", filterVersionKey: "v1", filterContentKey: "内容",
		}},
	}
	if err := store.Upsert(ctx, docFilterCollection, records); err != nil {
		t.Fatal(err)
	}
	return store
}

// TestNewDocumentFilterConfig 覆盖构造配置校验
func TestNewDocumentFilterConfig(t *testing.T) {
	t.Parallel()
	store := newFilterTestStore(t)
	full := testDocumentFilterConfig()
	if _, err := NewDocumentFilter(nil, full); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("nil store: %v", err)
	}
	cases := []struct {
		name string
		cfg  DocumentFilterConfig
	}{
		{"missing document key", func() DocumentFilterConfig { c := full; c.DocumentKey = ""; return c }()},
		{"missing index key", func() DocumentFilterConfig { c := full; c.IndexKey = ""; return c }()},
		{"missing count key", func() DocumentFilterConfig { c := full; c.CountKey = ""; return c }()},
		{"missing version key", func() DocumentFilterConfig { c := full; c.VersionKey = ""; return c }()},
		{"missing first chunk id", func() DocumentFilterConfig { c := full; c.FirstChunkID = nil; return c }()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewDocumentFilter(store, tc.cfg); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("期望 ErrInvalidArgument，实际 %v", err)
			}
		})
	}
	if _, err := NewDocumentFilter(store, full); err != nil {
		t.Fatalf("合法配置被拒: %v", err)
	}
}

// TestDocumentFilter 覆盖旧版本、越界序号、缺内容与无状态命中的过滤
func TestDocumentFilter(t *testing.T) {
	t.Parallel()
	store := newFilterTestStore(t)
	filter, err := NewDocumentFilter(store, testDocumentFilterConfig())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	current := filterHit("doc-a", 0, "v2", "块零")
	alsoCurrent := filterHit("doc-a", 1, "v2", "块一")
	staleVersion := filterHit("doc-a", 1, "v1", "旧版本")
	outOfRange := filterHit("doc-a", 2, "v2", "越界")
	noContent := filterHit("doc-a", 0, "v2", "")
	foreign := vectordb.Hit{ID: "foreign", Metadata: map[string]string{filterContentKey: "外部内容"}}
	hits := []vectordb.Hit{current, staleVersion, outOfRange, alsoCurrent, noContent, foreign}

	filtered, err := filter.Filter(ctx, docFilterCollection, hits)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 3 || filtered[0].ID != current.ID || filtered[1].ID != alsoCurrent.ID || filtered[2].ID != foreign.ID {
		t.Fatalf("过滤结果错误: %+v", filtered)
	}

	// 关闭内容检查：缺内容命中保留，其余过滤规则不变
	cfg := testDocumentFilterConfig()
	cfg.ContentKey = ""
	noContentFilter, err := NewDocumentFilter(store, cfg)
	if err != nil {
		t.Fatal(err)
	}
	filtered, err = noContentFilter.Filter(ctx, docFilterCollection, hits)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 4 {
		t.Fatalf("关闭内容检查后应保留缺内容命中: %+v", filtered)
	}
}

// TestDocumentFilterDegrade 覆盖辅助查询失败的降级与取消/集合消失的上抛
func TestDocumentFilterDegrade(t *testing.T) {
	t.Parallel()
	store := newFilterTestStore(t)
	hits := []vectordb.Hit{filterHit("doc-a", 0, "v2", "块零"), filterHit("doc-a", 1, "v1", "旧版本")}
	ctx := context.Background()

	// 普通辅助查询失败：降级为不过滤
	degraded, err := NewDocumentFilter(getFailStore{Store: store, err: errors.Sentinel(vectordb.ErrUnavailable)}, testDocumentFilterConfig())
	if err != nil {
		t.Fatal(err)
	}
	filtered, err := degraded.Filter(ctx, docFilterCollection, hits)
	if err != nil || len(filtered) != len(hits) {
		t.Fatalf("应降级为不过滤: %+v err=%v", filtered, err)
	}

	// 取消：上抛，不降级
	canceled, err := NewDocumentFilter(getFailStore{Store: store, err: context.Canceled}, testDocumentFilterConfig())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := canceled.Filter(ctx, docFilterCollection, hits); !errors.Is(err, context.Canceled) {
		t.Fatalf("取消应上抛，实际 %v", err)
	}

	// 集合消失：上抛，由检索层按空结果处理
	missing, err := NewDocumentFilter(getFailStore{Store: store, err: vectordb.ErrNotFound}, testDocumentFilterConfig())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := missing.Filter(ctx, docFilterCollection, hits); !errors.Is(err, vectordb.ErrNotFound) {
		t.Fatalf("集合消失应上抛 ErrNotFound，实际 %v", err)
	}
}

// TestDocumentFilterSkipsUnprobedDoc 覆盖首块主键推导为空时不发起状态查询
func TestDocumentFilterSkipsUnprobedDoc(t *testing.T) {
	t.Parallel()
	store := newFilterTestStore(t)
	counting := &countGetStore{Store: store}
	cfg := testDocumentFilterConfig()
	cfg.FirstChunkID = func(string) string { return "" }
	filter, err := NewDocumentFilter(counting, cfg)
	if err != nil {
		t.Fatal(err)
	}
	hits := []vectordb.Hit{filterHit("doc-a", 0, "v1", "块")}
	filtered, err := filter.Filter(context.Background(), docFilterCollection, hits)
	if err != nil || len(filtered) != 1 || counting.gets != 0 {
		t.Fatalf("不应查询状态: %+v gets=%d err=%v", filtered, counting.gets, err)
	}
}
