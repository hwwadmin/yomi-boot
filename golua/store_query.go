// Package golua StoreQuery 实现（只读 ScriptCache）
//
// @author zwei
package golua

import (
	"context"

	"gitee.com/hq_project/yomi-boot/errors"
)

var _ StoreQuery = (*storeQuery)(nil)

// storeQuery 是 StoreQuery 的默认实现
type storeQuery struct {
	cache ScriptCache
}

// newStoreQuery 创建只读查询实现
func newStoreQuery(cache ScriptCache) StoreQuery {
	return &storeQuery{cache: cache}
}

// Get 按 ID 读缓存
func (q *storeQuery) Get(ctx context.Context, id string) (*Script, error) {
	if q == nil || q.cache == nil {
		return nil, errors.New("golua: StoreQuery 未初始化")
	}
	return q.cache.Get(ctx, id)
}
