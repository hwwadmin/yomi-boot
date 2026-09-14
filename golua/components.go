// Package golua 组件装配
//
// @author zwei
package golua

import (
	"context"

	"gitee.com/hq_project/yomi-boot/errors"
)

// Components golua 运行时组件
type Components struct {
	Engine     Engine     // 执行入口
	StoreLoad  StoreLoad  // 灌库与热更新
	StoreQuery StoreQuery // 快照只读查询
}

// New 装配组件；默认执行一次 Refresh
func New(ctx context.Context, cfg Config, adapter StoreLoadAdapter) (*Components, error) {
	if adapter == nil {
		return nil, errors.New("golua: StoreLoadAdapter 不能为空")
	}
	cfg = cfg.withDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cache, err := newScriptCache(cfg)
	if err != nil {
		return nil, err
	}

	query := newStoreQuery(cache)
	eng, err := newEngine(cfg, cache)
	if err != nil {
		return nil, err
	}
	loader := newStoreLoad(adapter, cache, eng)

	if !cfg.SkipRefreshOnNew {
		if err := loader.Refresh(ctx); err != nil {
			_ = eng.Close()
			return nil, errors.Wrap(err, "golua: New 时 Refresh 失败")
		}
	}

	return &Components{
		Engine:     eng,
		StoreLoad:  loader,
		StoreQuery: query,
	}, nil
}

// newScriptCache 按 CacheBackend 创建快照缓存
func newScriptCache(cfg Config) (scriptCache, error) {
	switch cfg.CacheBackend {
	case CacheMemory:
		return newMemoryScriptCache(), nil
	case CacheRedis:
		return newRedisScriptCache(cfg.Redis, cfg.RedisKeyPrefix, cfg.DistLockFactory)
	default:
		return nil, errors.Newf("golua: 不支持的 CacheBackend: %s", cfg.CacheBackend)
	}
}

// Close 关闭组件
func (c *Components) Close() error {
	if c == nil || c.Engine == nil {
		return nil
	}
	return c.Engine.Close()
}
