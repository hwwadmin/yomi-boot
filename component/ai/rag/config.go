// 构造配置；本包不读 YAML。
//
// @author zwei

package rag

import (
	"gitee.com/hq_project/yomi-boot/component/ai/embedding"
	"gitee.com/hq_project/yomi-boot/component/ai/vectordb"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

const (
	maxDocIDLen     = 120 // 文档 ID 最大字节数，为 "文档ID#序号" 预留 8 字节
	maxChunksPerDoc = 10000
)

// Config 创建 Engine 的构造配置；只保留依赖，集合与运行期参数由调用传入
type Config struct {
	Embedder embedding.Client // Embedder 向量化客户端；必填
	Store    vectordb.Store   // Store 向量库；必填，各集合 Dimension 必须与向量化输出一致
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
