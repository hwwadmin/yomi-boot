// Package golua StoreLoad 实现
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

// storeLoad StoreLoad 默认实现
type storeLoad struct {
	adapter     StoreLoadAdapter
	cache       scriptCache
	invalidator ScriptInvalidator
	mu          sync.Mutex
}

// newStoreLoad 创建 StoreLoad
func newStoreLoad(adapter StoreLoadAdapter, cache scriptCache, invalidator ScriptInvalidator) StoreLoad {
	return &storeLoad{
		adapter:     adapter,
		cache:       cache,
		invalidator: invalidator,
	}
}

// Refresh 全量灌入；Ready 则跳过
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

// RefreshForce 强制全量回源
func (s *storeLoad) RefreshForce(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureReady(); err != nil {
		return err
	}
	return s.refreshAllLocked(ctx)
}

// RefreshScript 单脚本灌入；已存在则跳过
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

// RefreshScriptForce 强制单脚本回源；NotFound 时删除缓存
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

// ensureReady 校验依赖已就绪
func (s *storeLoad) ensureReady() error {
	if s == nil || s.cache == nil {
		return errors.New("golua: StoreLoad 未初始化")
	}
	if s.adapter == nil {
		return errors.New("golua: StoreLoadAdapter 不能为空")
	}
	return nil
}

// refreshAllLocked 全量回源并写入缓存
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
	s.afterCacheWrite(ctx, "")
	return nil
}

// refreshOneLocked 单脚本回源并写入或删除缓存
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
	s.afterCacheWrite(ctx, invalidateID)
	return nil
}

// afterCacheWrite 失效本地编译缓存并记录共享代际
func (s *storeLoad) afterCacheWrite(ctx context.Context, scriptID string) {
	if s.invalidator == nil {
		return
	}
	if scriptID == "" {
		s.invalidator.InvalidateAllScripts()
	} else {
		s.invalidator.InvalidateScripts(scriptID)
	}
	gen, ok, err := s.cache.CurrentGen(ctx)
	if err != nil || !ok {
		return
	}
	s.invalidator.ObserveGeneration(gen)
}
