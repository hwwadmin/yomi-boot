// Package vectordb New 按 Provider 分发实现。
//
// @author zwei
package vectordb

import (
	"strings"

	"gitee.com/hq_project/yomi-boot/errors"
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
	switch copied.Provider {
	case ProviderMemory:
		return newMemoryStore(copied), nil
	case ProviderMilvus:
		return newMilvusStore(copied)
	default:
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
}
