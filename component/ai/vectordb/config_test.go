// 构造配置与工厂单测。
//
// @author zwei

package vectordb

import (
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// TestVectorConfigValidate 覆盖 provider、度量、超时、容量与 Milvus 参数校验
func TestVectorConfigValidate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cfg  Config
		ok   bool
	}{
		{"memory default", Config{Provider: ProviderMemory}, true},
		{"milvus minimal", Config{Provider: ProviderMilvus, Milvus: MilvusConfig{Address: "127.0.0.1:19530"}}, true},
		{"unknown provider", Config{Provider: Provider("qdrant")}, false},
		{"unknown metric", Config{Provider: ProviderMemory, Metric: Metric("manhattan")}, false},
		{"negative timeout", Config{Provider: ProviderMemory, Timeout: -time.Second}, false},
		{"negative max records", Config{Provider: ProviderMemory, Memory: MemoryConfig{MaxRecords: -1}}, false},
		{"milvus blank address", Config{Provider: ProviderMilvus}, false},
		{"milvus unknown index", Config{Provider: ProviderMilvus, Milvus: MilvusConfig{Address: "127.0.0.1:19530", IndexType: IndexType("diskann")}}, false},
		{"milvus negative nlist", Config{Provider: ProviderMilvus, Milvus: MilvusConfig{Address: "127.0.0.1:19530", IndexNlist: -1}}, false},
		{"milvus negative m", Config{Provider: ProviderMilvus, Milvus: MilvusConfig{Address: "127.0.0.1:19530", IndexM: -1}}, false},
		{"milvus negative ef", Config{Provider: ProviderMilvus, Milvus: MilvusConfig{Address: "127.0.0.1:19530", IndexEFConstruct: -1}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.validate()
			if tc.ok && err != nil {
				t.Fatalf("err=%v", err)
			}
			if !tc.ok && !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

// TestVectorConfigDefaults 覆盖各组默认值回退
func TestVectorConfigDefaults(t *testing.T) {
	t.Parallel()
	if got := timeoutOrDefault(0); got != defaultTimeout {
		t.Fatalf("timeout=%v", got)
	}
	if got := timeoutOrDefault(time.Second); got != time.Second {
		t.Fatalf("timeout=%v", got)
	}
	if got := maxRecordsOrDefault(0); got != defaultMaxRecords {
		t.Fatalf("max records=%d", got)
	}
	if got := maxRecordsOrDefault(5); got != 5 {
		t.Fatalf("max records=%d", got)
	}
	metrics := map[Metric]Metric{
		"":         MetricCosine,
		" cosine ": MetricCosine,
		"l2":       MetricL2,
		" ip ":     MetricIP,
		MetricL2:   MetricL2,
	}
	for in, want := range metrics {
		if got := metricOrDefault(in); got != want {
			t.Fatalf("metricOrDefault(%q)=%q, want %q", in, got, want)
		}
	}
	indexes := map[IndexType]IndexType{
		"":          IndexHNSW,
		" hnsw ":    IndexHNSW,
		"ivf_flat":  IndexIVFFlat,
		"autoindex": IndexAuto,
		"flat":      IndexFlat,
	}
	for in, want := range indexes {
		if got := indexTypeOrDefault(in); got != want {
			t.Fatalf("indexTypeOrDefault(%q)=%q, want %q", in, got, want)
		}
	}
	if indexNlistOrDefault(0) != defaultIndexNlist || indexNlistOrDefault(7) != 7 {
		t.Fatal("nlist default mismatch")
	}
	if indexMOrDefault(0) != defaultIndexM || indexMOrDefault(7) != 7 {
		t.Fatal("m default mismatch")
	}
	if indexEFConstructOrDefault(0) != defaultIndexEFConstruct || indexEFConstructOrDefault(7) != 7 {
		t.Fatal("ef default mismatch")
	}
}

// TestNewStoreFactories 验证 memory 工厂归一化配置与非法 provider 拒绝
func TestNewStoreFactories(t *testing.T) {
	t.Parallel()
	store, err := New(Config{Provider: ProviderMemory, Metric: " l2 ", Timeout: 3 * time.Second, Memory: MemoryConfig{MaxRecords: 5}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	impl, ok := store.(*memoryStore)
	if !ok {
		t.Fatalf("type=%T", store)
	}
	if impl.metric != MetricL2 || impl.timeout != 3*time.Second || impl.maxRecords != 5 {
		t.Fatalf("store=%+v", impl)
	}
	if _, err := New(Config{Provider: Provider("qdrant")}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("err=%v", err)
	}
}

// TestVectorMustNew 验证成功返回 Store、失败 panic
func TestVectorMustNew(t *testing.T) {
	t.Parallel()
	store := MustNew(Config{Provider: ProviderMemory})
	if store == nil {
		t.Fatal("MustNew returned nil")
	}
	t.Cleanup(func() { _ = store.Close() })
	defer func() {
		if recover() == nil {
			t.Fatal("MustNew should panic on invalid config")
		}
	}()
	MustNew(Config{Provider: Provider("qdrant")})
}
