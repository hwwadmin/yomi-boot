// Package golua StoreQuery 实现
//
// @author zwei
package golua

import (
	"context"

	"gitee.com/hq_project/yomi-boot/errors"
)

var _ StoreQuery = (*storeQuery)(nil)

// storeQuery StoreQuery 默认实现
type storeQuery struct {
	cache scriptCache
}

// newStoreQuery 创建 StoreQuery
func newStoreQuery(cache scriptCache) StoreQuery {
	return &storeQuery{cache: cache}
}

// Get 按 ID 读快照
func (q *storeQuery) Get(ctx context.Context, id string) (*Script, error) {
	if q == nil || q.cache == nil {
		return nil, errors.New("golua: StoreQuery 未初始化")
	}
	return q.cache.Get(ctx, id)
}
