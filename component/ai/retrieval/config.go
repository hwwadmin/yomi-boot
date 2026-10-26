// 构造配置与公共入参校验；本包不读 YAML。
//
// @author zwei

package retrieval

import (
	"strings"

	"gitee.com/hq_project/yomi-boot/component/ai/embedding"
	"gitee.com/hq_project/yomi-boot/component/ai/vectordb"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// Config 创建 Engine 的构造配置；只保留依赖，集合与运行期参数由调用传入
type Config struct {
	Embedder embedding.Client // Embedder 向量化客户端；必填，用于查询向量化
	Store    vectordb.Store   // Store 向量库；必填，查询维度由 RetrieveRequest.Dimensions 传入并与集合维度一致
}

// validate 校验构造配置
func (c Config) validate() error {
	if c.Embedder == nil || c.Store == nil {
		return errors.Sentinel(ErrInvalidArgument)
	}
	return nil
}

// validateFilter 校验元数据等值过滤的键；非法请求须在向量化前拦截
func validateFilter(filter map[string]string) error {
	for k := range filter {
		key := strings.TrimSpace(k)
		if key == "" || strings.ContainsAny(key, `"\`) {
			return errors.Sentinel(ErrInvalidArgument)
		}
	}
	return nil
}
