// Package golua StoreQuery 实现（只读 Redis）
//
// @author zwei
package golua

import (
	"context"
	"encoding/json"

	"gitee.com/hq_project/yomi-boot/errors"

	redisv9 "github.com/redis/go-redis/v9"
)

var _ StoreQuery = (*storeQuery)(nil)

// storeQuery 是 StoreQuery 的 Redis 实现
type storeQuery struct {
	rdb  *redisv9.Client
	keys redisKeys
}

// NewStoreQuery 创建只读查询实现
func NewStoreQuery(rdb *redisv9.Client, keyPrefix string) StoreQuery {
	return &storeQuery{
		rdb:  rdb,
		keys: newRedisKeys(keyPrefix),
	}
}

// Get 按 ID 读缓存
func (q *storeQuery) Get(ctx context.Context, id string) (*Script, error) {
	if q == nil || q.rdb == nil {
		return nil, errors.New("golua: StoreQuery 未初始化")
	}
	raw, err := q.rdb.Get(ctx, q.keys.script(id)).Bytes()
	if err != nil {
		if errors.Is(err, redisv9.Nil) {
			return nil, errors.Sentinel(ErrNotFound)
		}
		return nil, errors.Wrap(err, "golua: 读取脚本缓存失败")
	}
	var script Script
	if err := json.Unmarshal(raw, &script); err != nil {
		return nil, errors.Wrap(err, "golua: 解析脚本缓存失败")
	}
	if err := script.normalize(); err != nil {
		return nil, err
	}
	return &script, nil
}
