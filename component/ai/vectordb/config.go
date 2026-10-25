// 构造配置；本包不读 YAML。
//
// @author zwei

package vectordb

import (
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// Provider 向量库实现标识
type Provider string

const (
	// ProviderMemory 进程内内存实现
	ProviderMemory Provider = "memory"
	// ProviderMilvus Milvus 实现
	ProviderMilvus Provider = "milvus"
)

// Metric 向量相似度度量；空则 COSINE
type Metric string

const (
	// MetricCosine 余弦相似度
	MetricCosine Metric = "COSINE"
	// MetricL2 欧氏距离（门面 Score 映射为越大越相似）
	MetricL2 Metric = "L2"
	// MetricIP 内积
	MetricIP Metric = "IP"
)

const (
	defaultTimeout          = 10 * time.Second
	compensateTimeout       = 3 * time.Second // 失败补偿的独立超时，避免失败路径过度延长
	defaultMaxRecords       = 100000
	defaultIndexNlist       = 1024
	defaultIndexM           = 16
	defaultIndexEFConstruct = 64
	maxDimension            = 32768
	// MaxTopK 是单次检索允许的最大 TopK
	MaxTopK         = 16384
	maxIDLen        = 128 // 主键最大字节数（Milvus VARCHAR max_length 按字节计）
	maxTextLen      = 65536
	maxNameLen      = 255
	upsertBatchSize = 500
)

// IndexType milvus 向量索引类型；空则 HNSW
type IndexType string

const (
	// IndexHNSW 图索引
	IndexHNSW IndexType = "HNSW"
	// IndexIVFFlat 倒排平坦索引
	IndexIVFFlat IndexType = "IVF_FLAT"
	// IndexAuto Milvus 自动索引
	IndexAuto IndexType = "AUTOINDEX"
	// IndexFlat 暴力扫描
	IndexFlat IndexType = "FLAT"
)

// Config 创建 Store 的构造配置；本包不读 YAML
type Config struct {
	Provider  Provider      // Provider 是 memory 或 milvus
	Dimension int           // Dimension 是本实例所有集合共用的向量维度；必须 > 0
	Metric    Metric        // Metric 空则 COSINE；仅 COSINE / L2 / IP
	Timeout   time.Duration // Timeout 是单次操作墙钟；0 表示 10s；小于 0 非法
	Memory    MemoryConfig  // Memory 仅 ProviderMemory 使用
	Milvus    MilvusConfig  // Milvus 仅 ProviderMilvus 使用
}

// MemoryConfig 内存实现配置
type MemoryConfig struct {
	MaxRecords int // MaxRecords 是 Store 内所有集合条数之和上限；0 表示 100000
}

// MilvusConfig Milvus 连接与索引配置
type MilvusConfig struct {
	Address          string    // Address 是 host:port，Provider=milvus 时必填
	DBName           string    // DBName 为空则使用 SDK 默认库
	Username         string    // Username 可选
	Password         string    // Password 可选；日志与错误正文不得回显完整值
	APIKey           string    // APIKey 可选；日志与错误正文不得回显完整值
	EnableTLS        bool      // EnableTLS 是否启用 TLS
	IndexType        IndexType // IndexType 空则 HNSW；仅 HNSW / IVF_FLAT / AUTOINDEX / FLAT
	IndexNlist       int       // IndexNlist 是 IVF_FLAT 的 nlist；0 表示 1024
	IndexM           int       // IndexM 是 HNSW 的 M；0 表示 16
	IndexEFConstruct int       // IndexEFConstruct 是 HNSW 的 efConstruction；0 表示 64
}

// timeoutOrDefault 返回操作超时；0 表示 10s
func timeoutOrDefault(d time.Duration) time.Duration {
	if d == 0 {
		return defaultTimeout
	}
	return d
}

// maxRecordsOrDefault 返回内存条数上限；0 表示默认
func maxRecordsOrDefault(n int) int {
	if n == 0 {
		return defaultMaxRecords
	}
	return n
}

// metricOrDefault 空度量回退 COSINE
func metricOrDefault(m Metric) Metric {
	if strings.TrimSpace(string(m)) == "" {
		return MetricCosine
	}
	return Metric(strings.ToUpper(strings.TrimSpace(string(m))))
}

// indexTypeOrDefault 空索引类型回退 HNSW
func indexTypeOrDefault(t IndexType) IndexType {
	if strings.TrimSpace(string(t)) == "" {
		return IndexHNSW
	}
	return IndexType(strings.ToUpper(strings.TrimSpace(string(t))))
}

// indexNlistOrDefault IVF_FLAT nlist；0 表示 1024
func indexNlistOrDefault(n int) int {
	if n == 0 {
		return defaultIndexNlist
	}
	return n
}

// indexMOrDefault HNSW M；0 表示 16
func indexMOrDefault(n int) int {
	if n == 0 {
		return defaultIndexM
	}
	return n
}

// indexEFConstructOrDefault HNSW efConstruction；0 表示 64
func indexEFConstructOrDefault(n int) int {
	if n == 0 {
		return defaultIndexEFConstruct
	}
	return n
}

// validate 校验构造配置
func (c Config) validate() error {
	switch c.Provider {
	case ProviderMemory, ProviderMilvus:
	default:
		return errors.Sentinel(ErrInvalidArgument)
	}
	if c.Dimension <= 0 || c.Dimension > maxDimension {
		return errors.Sentinel(ErrInvalidArgument)
	}
	switch metricOrDefault(c.Metric) {
	case MetricCosine, MetricL2, MetricIP:
	default:
		return errors.Sentinel(ErrInvalidArgument)
	}
	if c.Timeout < 0 {
		return errors.Sentinel(ErrInvalidArgument)
	}
	if c.Memory.MaxRecords < 0 {
		return errors.Sentinel(ErrInvalidArgument)
	}
	if c.Provider == ProviderMilvus {
		if strings.TrimSpace(c.Milvus.Address) == "" {
			return errors.Sentinel(ErrInvalidArgument)
		}
		switch indexTypeOrDefault(c.Milvus.IndexType) {
		case IndexHNSW, IndexIVFFlat, IndexAuto, IndexFlat:
		default:
			return errors.Sentinel(ErrInvalidArgument)
		}
		if c.Milvus.IndexNlist < 0 || c.Milvus.IndexM < 0 || c.Milvus.IndexEFConstruct < 0 {
			return errors.Sentinel(ErrInvalidArgument)
		}
	}
	return nil
}
