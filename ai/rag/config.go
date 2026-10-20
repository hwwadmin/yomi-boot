// Package rag 构造配置与切块默认值；本包不读 YAML。
//
// @author zwei
package rag

import (
	"regexp"
	"strings"

	"gitee.com/hq_project/yomi-boot/ai/embedding"
	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/vectordb"
)

const (
	defaultChunkSize    = 800
	defaultChunkOverlap = 100
	maxChunkSize        = 8192
	maxTopK             = 16384
	maxDocIDLen         = 120
	maxChunksPerDoc     = 10000
)

// collectionNameRe 集合名规则与 vectordb 保持一致
var collectionNameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,254}$`)

// Config 创建 Engine 的构造配置；只保留依赖，集合与运行期参数由调用传入
type Config struct {
	Embedder embedding.Client // Embedder 向量化客户端；必填
	Store    vectordb.Store   // Store 向量库；必填，各集合 Dimension 必须与向量化输出一致
}

// SplitConfig 内置切块策略配置
type SplitConfig struct {
	Strategy     SplitStrategy // Strategy 是切块策略；空则 recursive
	ChunkSize    int           // ChunkSize 是单块最大字符数（按 rune 计）；0 则 800；不得超过 8192
	ChunkOverlap int           // ChunkOverlap 是相邻块重叠字符数；0 则 min(100, ChunkSize/4)；必须小于 ChunkSize
	Separators   []string      // Separators 是 recursive 的分隔符优先级；其他策略不得设置；空则使用内置中英文分隔符
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
	n := strings.TrimSpace(name)
	if n == "" || !collectionNameRe.MatchString(n) {
		return "", errors.Sentinel(ErrInvalidArgument)
	}
	return n, nil
}
