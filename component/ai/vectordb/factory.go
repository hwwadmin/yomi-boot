// New 按 Provider 分发实现。
//
// @author zwei

package vectordb

import (
	"strings"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// New 校验配置并构造 Store；memory 不联网，milvus 会建连
func New(cfg Config) (Store, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	copied := cfg
	copied.Metric = metricOrDefault(cfg.Metric)
	copied.Milvus.Address = strings.TrimSpace(cfg.Milvus.Address)
	copied.Milvus.DBName = strings.TrimSpace(cfg.Milvus.DBName)
	copied.Milvus.Username = strings.TrimSpace(cfg.Milvus.Username)
	// APIKey 是令牌，修剪首尾空白；Password 有意不修剪，口令可合法包含首尾空格
	copied.Milvus.APIKey = strings.TrimSpace(cfg.Milvus.APIKey)
	copied.Milvus.IndexType = indexTypeOrDefault(cfg.Milvus.IndexType)
	copied.Milvus.IndexNlist = indexNlistOrDefault(cfg.Milvus.IndexNlist)
	copied.Milvus.IndexM = indexMOrDefault(cfg.Milvus.IndexM)
	copied.Milvus.IndexEFConstruct = indexEFConstructOrDefault(cfg.Milvus.IndexEFConstruct)
	switch copied.Provider {
	case ProviderMemory:
		return newMemoryStore(copied), nil
	case ProviderMilvus:
		return newMilvusStore(copied)
	default:
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
}
