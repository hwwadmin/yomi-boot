// 构造配置与切块默认值；本包不读 YAML。
//
// @author zwei

package rag

import (
	"gitee.com/hq_project/yomi-boot/ai/embedding"
	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/vectordb"
)

const (
	defaultChunkSize    = 800
	defaultChunkOverlap = 100
	maxChunkSize        = 8192
	maxDocIDLen         = 120 // 文档 ID 最大字节数，为 "文档ID#序号" 预留 8 字节
	maxChunksPerDoc     = 10000
)

// Config 创建 Engine 的构造配置；只保留依赖，集合与运行期参数由调用传入
type Config struct {
	Embedder embedding.Client // Embedder 向量化客户端；必填
	Store    vectordb.Store   // Store 向量库；必填，各集合 Dimension 必须与向量化输出一致
}

// SplitConfig 内置切块策略配置；只有 Strategy 对应的子配置生效，其余忽略
type SplitConfig struct {
	Strategy  SplitStrategy   // Strategy 是切块策略；空则 recursive
	Recursive RecursiveConfig // Recursive 仅在 Strategy 为 recursive 时生效
	Fixed     FixedConfig     // Fixed 仅在 Strategy 为 fixed 时生效
	Sentence  SentenceConfig  // Sentence 仅在 Strategy 为 sentence 时生效
	Markdown  MarkdownConfig  // Markdown 仅在 Strategy 为 markdown 时生效
}

// RecursiveConfig recursive 策略配置
type RecursiveConfig struct {
	ChunkSize    int      // ChunkSize 是单块最大字符数（按 rune 计）；0 则 800；不得超过 8192
	ChunkOverlap int      // ChunkOverlap 是相邻块重叠字符数；0 则 min(100, ChunkSize/4)；必须小于 ChunkSize
	Separators   []string // Separators 是分隔符优先级；空则使用内置中英文分隔符；元素不得为空串
}

// FixedConfig fixed 策略配置
type FixedConfig struct {
	ChunkSize    int // ChunkSize 是单块最大字符数（按 rune 计）；0 则 800；不得超过 8192
	ChunkOverlap int // ChunkOverlap 是相邻块重叠字符数；0 则 min(100, ChunkSize/4)；必须小于 ChunkSize
}

// SentenceConfig sentence 策略配置
type SentenceConfig struct {
	ChunkSize    int      // ChunkSize 是单块最大字符数（按 rune 计）；0 则 800；不得超过 8192
	ChunkOverlap int      // ChunkOverlap 是相邻块重叠字符数；0 则 min(100, ChunkSize/4)；必须小于 ChunkSize
	Separators   []string // Separators 是句末分隔符；空则使用内置中英文句末标点；元素不得为空串
}

// MarkdownConfig markdown 策略配置
type MarkdownConfig struct {
	ChunkSize       int // ChunkSize 是单块最大字符数（按 rune 计）；0 则 800；不得超过 8192
	ChunkOverlap    int // ChunkOverlap 是相邻块重叠字符数；0 则 min(100, ChunkSize/4)；必须小于 ChunkSize
	MaxHeadingLevel int // MaxHeadingLevel 是参与分节的最大标题层级；0 则 6；取值 1..6
}

// validate 校验构造配置
func (c Config) validate() error {
	if c.Embedder == nil || c.Store == nil {
		return errors.Sentinel(ErrInvalidArgument)
	}
	return nil
}

// normalizeCollection 修剪并校验集合名；空串或非法命名返回 ErrInvalidArgument
func normalizeCollection(name string) (string, error) {
	n, err := vectordb.NormalizeCollectionName(name)
	if err != nil {
		return "", errors.Sentinel(ErrInvalidArgument)
	}
	return n, nil
}
