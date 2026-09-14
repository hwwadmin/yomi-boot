// Package golua StoreLoad 实现（Adapter → scriptCache + 本地失效）
//
// @author zwei
package golua

import (
	"context"
	"strings"
	"sync"

	"gitee.com/hq_project/yomi-boot/errors"
)

var _ StoreLoad = (*storeLoad)(nil)

// storeLoad 是 StoreLoad 的默认实现
type storeLoad struct {
	adapter     StoreLoadAdapter
	cache       scriptCache
	invalidator ScriptInvalidator
	mu          sync.Mutex
}

// newStoreLoad 创建缓存灌入实现
func newStoreLoad(adapter StoreLoadAdapter, cache scriptCache, invalidator ScriptInvalidator) StoreLoad {
	return &storeLoad{
		adapter:     adapter,
		cache:       cache,
		invalidator: invalidator,
	}
}

// Refresh 普通全量：缓存 Ready 则跳过
func (s *storeLoad) Refresh(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureReady(); err != nil {
		return err
	}
	ok, err := s.cache.Ready(ctx)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	return s.refreshAllLocked(ctx)
}

// RefreshForce 强制全量
func (s *storeLoad) RefreshForce(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureReady(); err != nil {
		return err
	}
	return s.refreshAllLocked(ctx)
}

// RefreshScript 普通单脚本：已存在则跳过
func (s *storeLoad) RefreshScript(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureReady(); err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("golua: 脚本 id 不能为空")
	}
	ok, err := s.cache.Has(ctx, id)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	return s.refreshOneLocked(ctx, id, false)
}

// RefreshScriptForce 强制单脚本；数据源 NotFound 则删缓存
func (s *storeLoad) RefreshScriptForce(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureReady(); err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("golua: 脚本 id 不能为空")
	}
	return s.refreshOneLocked(ctx, id, true)
}

// ensureReady 校验依赖
func (s *storeLoad) ensureReady() error {
	if s == nil || s.cache == nil {
		return errors.New("golua: StoreLoad 未初始化")
	}
	if s.adapter == nil {
		return errors.New("golua: StoreLoadAdapter 不能为空")
	}
	return nil
}

// refreshAllLocked 全量回源写入缓存（ReplaceAllFrom：Redis 下 Load 与写入同一把 distlock）
func (s *storeLoad) refreshAllLocked(ctx context.Context) error {
	err := s.cache.ReplaceAllFrom(ctx, func(ctx context.Context) ([]*Script, error) {
		scripts, err := s.adapter.Load(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "golua: Adapter.Load 失败")
		}
		return scripts, nil
	})
	if err != nil {
		return err
	}
	if s.invalidator != nil {
		s.invalidator.InvalidateAllScripts()
	}
	return nil
}

// refreshOneLocked 单脚本回源（UpsertFrom：Redis 下 LoadByID 与 Put/Remove 同一把 distlock）
func (s *storeLoad) refreshOneLocked(ctx context.Context, id string, deleteOnNotFound bool) error {
	invalidateID := id
	err := s.cache.UpsertFrom(ctx, id, deleteOnNotFound, func(ctx context.Context) (*Script, error) {
		script, err := s.adapter.LoadByID(ctx, id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return nil, errors.Sentinel(ErrNotFound)
			}
			return nil, errors.Wrap(err, "golua: Adapter.LoadByID 失败")
		}
		if script == nil {
			return nil, errors.New("golua: Adapter.LoadByID 返回空脚本")
		}
		invalidateID = script.ID
		return script, nil
	})
	if err != nil {
		return err
	}
	if s.invalidator != nil {
		s.invalidator.InvalidateScripts(invalidateID)
	}
	return nil
}
