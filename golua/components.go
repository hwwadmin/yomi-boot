// Package golua 组件装配
//
// @author zwei
package golua

import (
	"context"

	"gitee.com/hq_project/yomi-boot/errors"
)

// Components 可注入业务的 golua 运行时组件
type Components struct {
	Engine     Engine     // Engine 是脚本执行入口
	StoreLoad  StoreLoad  // StoreLoad 供业务灌库与热更新
	StoreQuery StoreQuery // StoreQuery 是只读缓存视图（一般无需业务直调）
}

// New 校验配置并装配 Engine / StoreLoad / StoreQuery。
// 默认会执行一次普通 Refresh；cfg.SkipRefreshOnNew=true 时可跳过。
func New(ctx context.Context, cfg Config, adapter StoreLoadAdapter, cache ScriptCache) (*Components, error) {
	if adapter == nil {
		return nil, errors.New("golua: StoreLoadAdapter 不能为空")
	}
	if cache == nil {
		return nil, errors.New("golua: ScriptCache 不能为空")
	}
	cfg = cfg.withDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	query := newStoreQuery(cache)
	eng, err := newEngine(cfg, query)
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

// Close 关闭引擎
func (c *Components) Close() error {
	if c == nil || c.Engine == nil {
		return nil
	}
	return c.Engine.Close()
}
