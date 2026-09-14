// Package golua 组件装配
//
// @author zwei
package golua

import (
	"context"

	"gitee.com/hq_project/yomi-boot/errors"

	redisv9 "github.com/redis/go-redis/v9"
)

// Components 可注入业务的 golua 运行时组件
type Components struct {
	Engine     Engine     // Engine 是脚本执行入口
	StoreLoad  StoreLoad  // StoreLoad 供业务启动灌库与热更新
	StoreQuery StoreQuery // StoreQuery 是只读缓存视图（一般无需业务直调）
}

// New 校验配置并装配 Engine / StoreLoad / StoreQuery。
// enabled=false 时返回 ErrDisabled。
// refresh_on_start=true 时于创建后执行一次普通 Refresh。
func New(ctx context.Context, cfg Config, adapter StoreLoadAdapter, rdb *redisv9.Client) (*Components, error) {
	if !cfg.EnabledOrDefault() {
		return nil, errors.Sentinel(ErrDisabled)
	}
	if adapter == nil {
		return nil, errors.New("golua: StoreLoadAdapter 不能为空")
	}
	if rdb == nil {
		return nil, errors.New("golua: redis client 不能为空")
	}
	cfg = cfg.withDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	query := NewStoreQuery(rdb, cfg.RedisKeyPrefix)
	eng, err := newEngine(cfg, query)
	if err != nil {
		return nil, err
	}
	loader := NewStoreLoad(adapter, rdb, cfg.RedisKeyPrefix, eng)

	if cfg.RefreshOnStartOrDefault() {
		if err := loader.Refresh(ctx); err != nil {
			_ = eng.Close()
			return nil, errors.Wrap(err, "golua: 启动 Refresh 失败")
		}
	}

	return &Components{
		Engine:     eng,
		StoreLoad:  loader,
		StoreQuery: query,
	}, nil
}

// Close 关闭引擎
func (c *Components) Close() error {
	if c == nil || c.Engine == nil {
		return nil
	}
	return c.Engine.Close()
}
