// Package vectordb Milvus 契约测试；未设置 VECTORDB_MILVUS_ADDR 时跳过。
//
// @author zwei
package vectordb

import (
	"os"
	"testing"

	"github.com/milvus-io/milvus/client/v2/column"
	"github.com/milvus-io/milvus/client/v2/milvusclient"

	"gitee.com/hq_project/yomi-boot/errors"
)

// TestStoreContractMilvus 对真实 Milvus 跑与 memory 相同的契约
func TestStoreContractMilvus(t *testing.T) {
	addr := os.Getenv("VECTORDB_MILVUS_ADDR")
	if addr == "" {
		t.Skip("VECTORDB_MILVUS_ADDR 未设置")
	}
	store, err := New(Config{
		Provider:  ProviderMilvus,
		Dimension: 2,
		Milvus:    MilvusConfig{Address: addr},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	runStoreContract(t, store, 2)
}

// TestIDsFromResultSetMissingColumn 验证缺少主键列时返回 ErrUnavailable
func TestIDsFromResultSetMissingColumn(t *testing.T) {
	t.Parallel()
	rs := milvusclient.ResultSet{ResultCount: 1}
	if _, err := resultSetToHits(&rs, MetricCosine); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("hits: %v", err)
	}
	if _, err := resultSetToRecords(&rs); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("records: %v", err)
	}
}

// TestMetadataAtInvalidJSON 验证损坏的 JSON 元数据不得静默丢字段
func TestMetadataAtInvalidJSON(t *testing.T) {
	t.Parallel()
	col := column.NewColumnJSONBytes(fieldMetadata, [][]byte{[]byte(`{`)})
	if _, err := metadataAt(col, 0); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
}
