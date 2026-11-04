// Milvus 实现中可离线验证的纯函数单测。
//
// @author zwei

package vectordb

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/hwwadmin/yomi-boot/component/core/errors"

	"github.com/milvus-io/milvus/client/v2/column"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

// TestToEntityMetric 验证门面度量到 SDK 度量的映射
func TestToEntityMetric(t *testing.T) {
	t.Parallel()
	cases := map[Metric]entity.MetricType{
		"":         entity.COSINE,
		" cosine ": entity.COSINE,
		MetricL2:   entity.L2,
		"l2":       entity.L2,
		MetricIP:   entity.IP,
		" ip ":     entity.IP,
	}
	for in, want := range cases {
		if got := toEntityMetric(in); got != want {
			t.Fatalf("toEntityMetric(%q)=%v, want %v", in, got, want)
		}
	}
}

// TestMetadataFilterExpr 验证等值过滤表达式与字面量转义
func TestMetadataFilterExpr(t *testing.T) {
	t.Parallel()
	if _, err := metadataFilterExpr(nil); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("empty filter err=%v", err)
	}
	expr, err := metadataFilterExpr(map[string]string{"k": "v"})
	if err != nil || expr != `metadata["k"] == "v"` {
		t.Fatalf("expr=%q err=%v", expr, err)
	}
	expr, err = metadataFilterExpr(map[string]string{"a": "1", "b": "2"})
	if err != nil {
		t.Fatalf("multi err=%v", err)
	}
	if !strings.Contains(expr, `metadata["a"] == "1"`) || !strings.Contains(expr, `metadata["b"] == "2"`) || !strings.Contains(expr, " && ") {
		t.Fatalf("expr=%q", expr)
	}
	if _, err := metadataFilterExpr(map[string]string{`a"b`: "v"}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("bad key err=%v", err)
	}
	expr, err = metadataFilterExpr(map[string]string{"k": `a"b\c`})
	if err != nil || !strings.Contains(expr, `"a\"b\\c"`) {
		t.Fatalf("escaped expr=%q err=%v", expr, err)
	}
}

// TestQuoteMilvusString 验证引号与反斜杠转义
func TestQuoteMilvusString(t *testing.T) {
	t.Parallel()
	if got := quoteMilvusString(`a"b\c`); got != `"a\"b\\c"` {
		t.Fatalf("got %q", got)
	}
	if got := quoteMilvusString("plain"); got != `"plain"` {
		t.Fatalf("got %q", got)
	}
}

// TestMarshalMetadata 验证元数据 JSON 编码与空值处理
func TestMarshalMetadata(t *testing.T) {
	t.Parallel()
	raw, err := marshalMetadata(nil)
	if err != nil || string(raw) != "{}" {
		t.Fatalf("nil raw=%s err=%v", raw, err)
	}
	raw, err = marshalMetadata(map[string]string{" k ": "v"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["k"] != "v" || len(got) != 1 {
		t.Fatalf("got=%v", got)
	}
}

// TestSchemaDimension 验证从 schema 读取向量维度及异常形态
func TestSchemaDimension(t *testing.T) {
	t.Parallel()
	if _, err := schemaDimension(nil); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil schema err=%v", err)
	}
	if _, err := schemaDimension(entity.NewSchema()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("empty schema err=%v", err)
	}
	noVector := entity.NewSchema().WithField(
		entity.NewField().WithName(fieldID).WithDataType(entity.FieldTypeVarChar).WithMaxLength(maxIDLen),
	)
	if _, err := schemaDimension(noVector); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("no vector err=%v", err)
	}
	zeroDim := entity.NewSchema().WithField(
		entity.NewField().WithName(fieldVector).WithDataType(entity.FieldTypeFloatVector).WithDim(0),
	)
	if _, err := schemaDimension(zeroDim); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("zero dim err=%v", err)
	}
	valid := entity.NewSchema().WithField(
		entity.NewField().WithName(fieldVector).WithDataType(entity.FieldTypeFloatVector).WithDim(8),
	)
	dim, err := schemaDimension(valid)
	if err != nil || dim != 8 {
		t.Fatalf("dim=%d err=%v", dim, err)
	}
}

// TestMapMilvusErr 验证 SDK 错误文本到哨兵的映射
func TestMapMilvusErr(t *testing.T) {
	t.Parallel()
	if err := mapMilvusErr(nil); err != nil {
		t.Fatalf("nil=%v", err)
	}
	cases := []struct {
		msg  string
		want error
	}{
		{"collection already exist", ErrAlreadyExists},
		{"collection not found", ErrNotFound},
		{"database does not exist", ErrNotFound},
		{"database doesn't exist", ErrNotFound},
		{"connection refused", ErrUnavailable},
	}
	for _, tc := range cases {
		cause := errors.New(tc.msg)
		err := mapMilvusErr(cause)
		if !errors.Is(err, tc.want) || !errors.Is(err, cause) {
			t.Fatalf("%q: err=%v", tc.msg, err)
		}
	}
	if err := mapMilvusErr(context.Canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled err=%v", err)
	}
}

// newResultSetForTest 构造两条命中结果集
func newResultSetForTest() *milvusclient.ResultSet {
	return &milvusclient.ResultSet{
		ResultCount: 2,
		IDs:         column.NewColumnVarChar(fieldID, []string{"a", "b"}),
		Fields: milvusclient.DataSet{
			column.NewColumnJSONBytes(fieldMetadata, [][]byte{[]byte(`{"k":"v"}`), []byte(`null`)}),
			column.NewColumnFloatVector(fieldVector, 2, [][]float32{{1, 0}, {0, 1}}),
			column.NewColumnVarChar(fieldText, []string{"ta", "tb"}),
		},
		Scores: []float32{0.8, 0.2},
	}
}

// TestResultSetConversions 验证结果集到 Hit/Record 的转换与边界
func TestResultSetConversions(t *testing.T) {
	t.Parallel()
	rs := newResultSetForTest()
	ids, err := idsFromResultSet(rs)
	if err != nil || len(ids) != 2 || ids[0] != "a" || ids[1] != "b" {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
	if got, err := idsFromResultSet(nil); got != nil || err != nil {
		t.Fatalf("nil ids=%v err=%v", got, err)
	}

	hits, err := resultSetToHits(rs, MetricL2)
	if err != nil {
		t.Fatalf("hits err=%v", err)
	}
	if len(hits) != 2 || hits[0].ID != "a" || hits[0].Metadata["k"] != "v" || len(hits[1].Metadata) != 0 {
		t.Fatalf("hits=%+v", hits)
	}
	if math.Abs(float64(hits[0].Score-1.0/1.8)) > 1e-6 || math.Abs(float64(hits[1].Score-1.0/1.2)) > 1e-6 {
		t.Fatalf("L2 scores=%+v", hits)
	}

	rawHits, err := hitsWithScore(rs, nil)
	if err != nil || rawHits[0].Score != 0.8 || rawHits[1].Score != 0.2 {
		t.Fatalf("raw hits=%+v err=%v", rawHits, err)
	}
	shortScores, err := hitsWithScore(&milvusclient.ResultSet{
		ResultCount: 2,
		IDs:         column.NewColumnVarChar(fieldID, []string{"a", "b"}),
		Scores:      []float32{0.5},
	}, nil)
	if err != nil || shortScores[1].Score != 0 {
		t.Fatalf("short scores=%+v err=%v", shortScores, err)
	}

	records, err := resultSetToRecords(rs)
	if err != nil {
		t.Fatalf("records err=%v", err)
	}
	if len(records) != 2 || records[0].ID != "a" || records[0].Text != "ta" || records[0].Vector[0] != 1 || records[0].Metadata["k"] != "v" {
		t.Fatalf("records=%+v", records)
	}
	if got, err := resultSetToHits(nil, MetricCosine); err != nil || len(got) != 0 {
		t.Fatalf("nil hits=%+v err=%v", got, err)
	}
	if got, err := resultSetToRecords(&milvusclient.ResultSet{}); err != nil || len(got) != 0 {
		t.Fatalf("empty records=%+v err=%v", got, err)
	}

	byIDField := &milvusclient.ResultSet{
		ResultCount: 1,
		Fields: milvusclient.DataSet{
			column.NewColumnVarChar(fieldID, []string{"x"}),
		},
	}
	ids, err = idsFromResultSet(byIDField)
	if err != nil || len(ids) != 1 || ids[0] != "x" {
		t.Fatalf("id column ids=%v err=%v", ids, err)
	}
	if _, err := idsFromResultSet(&milvusclient.ResultSet{ResultCount: 1}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("missing id column err=%v", err)
	}
}

// TestColumnAccessors 覆盖列读取的缺失、越界与类型分支
func TestColumnAccessors(t *testing.T) {
	t.Parallel()
	if got, err := textAt(nil, 0); got != "" || err != nil {
		t.Fatalf("nil text=%q err=%v", got, err)
	}
	textCol := column.NewColumnVarChar(fieldText, []string{"x"})
	if _, err := textAt(textCol, 1); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("text out of range err=%v", err)
	}
	if _, err := vectorAt(nil, 0); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil vector err=%v", err)
	}
	vecCol := column.NewColumnFloatVector(fieldVector, 2, [][]float32{{1, 2}})
	vec, err := vectorAt(vecCol, 0)
	if err != nil || len(vec) != 2 || vec[0] != 1 || vec[1] != 2 {
		t.Fatalf("vec=%v err=%v", vec, err)
	}
	if _, err := vectorAt(column.NewColumnInt64("i", []int64{1}), 0); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("wrong vector type err=%v", err)
	}
	if got, err := metadataAt(nil, 0); got != nil || err != nil {
		t.Fatalf("nil metadata=%v err=%v", got, err)
	}
	md, err := metadataAt(column.NewColumnVarChar(fieldMetadata, []string{`{"k":"v"}`}), 0)
	if err != nil || md["k"] != "v" {
		t.Fatalf("string metadata=%v err=%v", md, err)
	}
	if got, err := metadataAt(column.NewColumnJSONBytes(fieldMetadata, [][]byte{[]byte("null")}), 0); got != nil || err != nil {
		t.Fatalf("null metadata=%v err=%v", got, err)
	}
	if got, err := metadataAt(column.NewColumnJSONBytes(fieldMetadata, [][]byte{nil}), 0); got != nil || err != nil {
		t.Fatalf("empty metadata=%v err=%v", got, err)
	}
	if _, err := metadataAt(column.NewColumnJSONBytes(fieldMetadata, [][]byte{[]byte("{bad")}), 0); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("bad json err=%v", err)
	}
	if got, err := metadataAt(column.NewColumnInt64("i", []int64{1}), 0); got != nil || err != nil {
		t.Fatalf("wrong metadata type=%v err=%v", got, err)
	}
	if _, err := metadataAt(column.NewColumnVarChar(fieldMetadata, []string{"x"}), 1); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("metadata out of range err=%v", err)
	}
}

// TestMilvusStoreLifecycle 验证未关闭判定与空客户端关闭
func TestMilvusStoreLifecycle(t *testing.T) {
	t.Parallel()
	store := &milvusStore{timeout: time.Second}
	if err := store.errIfClosed(); err != nil {
		t.Fatalf("errIfClosed: %v", err)
	}
	ctx, cancel, err := store.begin(nilContext())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer cancel()
	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("begin did not apply timeout")
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("repeat Close: %v", err)
	}
}

// TestResultSetConversionErrors 覆盖结果集读取失败的各类分支
func TestResultSetConversionErrors(t *testing.T) {
	t.Parallel()
	noID := &milvusclient.ResultSet{ResultCount: 1}
	if _, err := hitsWithScore(noID, nil); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("hits missing id err=%v", err)
	}
	noVector := &milvusclient.ResultSet{
		ResultCount: 1,
		IDs:         column.NewColumnVarChar(fieldID, []string{"a"}),
	}
	if _, err := resultSetToRecords(noVector); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("records missing vector err=%v", err)
	}
	emptyText := &milvusclient.ResultSet{
		ResultCount: 1,
		IDs:         column.NewColumnVarChar(fieldID, []string{"a"}),
		Fields: milvusclient.DataSet{
			column.NewColumnFloatVector(fieldVector, 2, [][]float32{{1, 0}}),
			column.NewColumnVarChar(fieldText, nil),
		},
	}
	if _, err := resultSetToRecords(emptyText); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("records text err=%v", err)
	}
	emptyMeta := &milvusclient.ResultSet{
		ResultCount: 1,
		IDs:         column.NewColumnVarChar(fieldID, []string{"a"}),
		Fields: milvusclient.DataSet{
			column.NewColumnFloatVector(fieldVector, 2, [][]float32{{1, 0}}),
			column.NewColumnVarChar(fieldText, []string{"t"}),
			column.NewColumnJSONBytes(fieldMetadata, nil),
		},
	}
	if _, err := resultSetToRecords(emptyMeta); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("records metadata err=%v", err)
	}
	if _, err := hitsWithScore(emptyMeta, nil); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("hits metadata err=%v", err)
	}
	if _, err := resultSetToRecords(noID); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("records id err=%v", err)
	}
	vecCol := column.NewColumnFloatVector(fieldVector, 2, [][]float32{{1, 0}})
	if _, err := vectorAt(vecCol, 3); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("vector out of range err=%v", err)
	}
}

// TestMilvusStoreClosedOperations 验证关闭后所有入口在触网前返回 ErrClosed
func TestMilvusStoreClosedOperations(t *testing.T) {
	t.Parallel()
	store := &milvusStore{closed: true}
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

// TestMilvusDimensionCache 验证维度缓存命中与失效
func TestMilvusDimensionCache(t *testing.T) {
	t.Parallel()
	store := &milvusStore{dims: map[string]int{"c": 8}}
	dim, err := store.collectionDimension(context.Background(), "c")
	if err != nil || dim != 8 {
		t.Fatalf("dim=%d err=%v", dim, err)
	}
	store.forgetDimension("c")
	if _, ok := store.dims["c"]; ok {
		t.Fatal("dimension cache not cleared")
	}
}

// TestMilvusNewVectorIndex 验证各索引类型都能构造
func TestMilvusNewVectorIndex(t *testing.T) {
	t.Parallel()
	for _, typ := range []IndexType{IndexHNSW, IndexIVFFlat, IndexAuto, IndexFlat, ""} {
		store := &milvusStore{
			metric:           MetricCosine,
			indexType:        typ,
			indexNlist:       8,
			indexM:           4,
			indexEFConstruct: 32,
		}
		if idx := store.newVectorIndex(); idx == nil {
			t.Fatalf("index type %q returned nil", typ)
		}
	}
}

// TestNewMilvusStoreUnavailable 验证不可达地址返回 ErrUnavailable
func TestNewMilvusStoreUnavailable(t *testing.T) {
	t.Parallel()
	store, err := New(Config{
		Provider: ProviderMilvus,
		Timeout:  200 * time.Millisecond,
		Milvus:   MilvusConfig{Address: "127.0.0.1:1"},
	})
	if !errors.Is(err, ErrUnavailable) || store != nil {
		t.Fatalf("store=%v err=%v", store, err)
	}
}
