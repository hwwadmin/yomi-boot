// Package golua StoreLoad 实现（Adapter → Redis + 本地失效）
//
// @author zwei
package golua

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"gitee.com/hq_project/yomi-boot/errors"

	redisv9 "github.com/redis/go-redis/v9"
)

var _ StoreLoad = (*storeLoad)(nil)

// storeLoad 是 StoreLoad 的默认实现（Adapter 回源 + Redis 写入）
type storeLoad struct {
	adapter     StoreLoadAdapter
	rdb         *redisv9.Client
	keys        redisKeys
	invalidator ScriptInvalidator
	mu          sync.Mutex
}

// NewStoreLoad 创建缓存灌入实现
func NewStoreLoad(adapter StoreLoadAdapter, rdb *redisv9.Client, keyPrefix string, invalidator ScriptInvalidator) StoreLoad {
	return &storeLoad{
		adapter:     adapter,
		rdb:         rdb,
		keys:        newRedisKeys(keyPrefix),
		invalidator: invalidator,
	}
}

// Refresh 普通全量：current_gen 存在且 index 非空则跳过
func (s *storeLoad) Refresh(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureReady(); err != nil {
		return err
	}
	ok, err := s.cacheReady(ctx)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	return s.refreshAllLocked(ctx, true)
}

// RefreshForce 强制全量
func (s *storeLoad) RefreshForce(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureReady(); err != nil {
		return err
	}
	return s.refreshAllLocked(ctx, true)
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
	n, err := s.rdb.Exists(ctx, s.keys.script(id)).Result()
	if err != nil {
		return errors.Wrap(err, "golua: 检查脚本缓存失败")
	}
	if n > 0 {
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

// ensureReady 校验 StoreLoad 依赖已注入
func (s *storeLoad) ensureReady() error {
	if s == nil || s.rdb == nil {
		return errors.New("golua: StoreLoad 未初始化")
	}
	if s.adapter == nil {
		return errors.New("golua: StoreLoadAdapter 不能为空")
	}
	return nil
}

// cacheReady 以 current_gen 存在且 index 非空判非空
func (s *storeLoad) cacheReady(ctx context.Context) (bool, error) {
	gen, err := s.rdb.Exists(ctx, s.keys.gen()).Result()
	if err != nil {
		return false, errors.Wrap(err, "golua: 检查 current_gen 失败")
	}
	if gen == 0 {
		return false, nil
	}
	n, err := s.rdb.SCard(ctx, s.keys.index()).Result()
	if err != nil {
		return false, errors.Wrap(err, "golua: 检查 index 失败")
	}
	return n > 0, nil
}

// refreshAllLocked 全量回源写入 Redis；调用方须持有 mu
func (s *storeLoad) refreshAllLocked(ctx context.Context, invalidate bool) error {
	scripts, err := s.adapter.Load(ctx)
	if err != nil {
		return errors.Wrap(err, "golua: Adapter.Load 失败")
	}
	normalized := make([]*Script, 0, len(scripts))
	idSet := make(map[string]struct{}, len(scripts))
	for _, sc := range scripts {
		if sc == nil {
			continue
		}
		cp := *sc
		if err := cp.normalize(); err != nil {
			return err
		}
		if _, dup := idSet[cp.ID]; dup {
			return errors.Newf("golua: 脚本 id 重复: %s", cp.ID)
		}
		idSet[cp.ID] = struct{}{}
		normalized = append(normalized, &cp)
	}

	pipe := s.rdb.TxPipeline()
	newIDs := make([]any, 0, len(normalized))
	for _, sc := range normalized {
		payload, err := json.Marshal(sc)
		if err != nil {
			return errors.Wrap(err, "golua: 序列化脚本失败")
		}
		pipe.Set(ctx, s.keys.script(sc.ID), payload, 0)
		newIDs = append(newIDs, sc.ID)
	}

	oldIDs, err := s.rdb.SMembers(ctx, s.keys.index()).Result()
	if err != nil && !errors.Is(err, redisv9.Nil) {
		return errors.Wrap(err, "golua: 读取旧 index 失败")
	}
	for _, old := range oldIDs {
		if _, ok := idSet[old]; !ok {
			pipe.Del(ctx, s.keys.script(old))
			pipe.SRem(ctx, s.keys.index(), old)
		}
	}
	if len(newIDs) > 0 {
		pipe.SAdd(ctx, s.keys.index(), newIDs...)
	}
	pipe.Incr(ctx, s.keys.gen())

	if _, err := pipe.Exec(ctx); err != nil {
		return errors.Wrap(err, "golua: 写入脚本缓存失败")
	}

	if invalidate && s.invalidator != nil {
		s.invalidator.InvalidateAllScripts()
	}
	return nil
}

// refreshOneLocked 单脚本回源；deleteOnNotFound 为 true 且数据源 NotFound 时删缓存
func (s *storeLoad) refreshOneLocked(ctx context.Context, id string, deleteOnNotFound bool) error {
	script, err := s.adapter.LoadByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			if !deleteOnNotFound {
				return errors.Sentinel(ErrNotFound)
			}
			pipe := s.rdb.TxPipeline()
			pipe.Del(ctx, s.keys.script(id))
			pipe.SRem(ctx, s.keys.index(), id)
			pipe.Incr(ctx, s.keys.gen())
			if _, err := pipe.Exec(ctx); err != nil {
				return errors.Wrap(err, "golua: 删除脚本缓存失败")
			}
			if s.invalidator != nil {
				s.invalidator.InvalidateScripts(id)
			}
			return nil
		}
		return errors.Wrap(err, "golua: Adapter.LoadByID 失败")
	}
	if script == nil {
		return errors.New("golua: Adapter.LoadByID 返回空脚本")
	}
	cp := *script
	if err := cp.normalize(); err != nil {
		return err
	}
	payload, err := json.Marshal(&cp)
	if err != nil {
		return errors.Wrap(err, "golua: 序列化脚本失败")
	}
	pipe := s.rdb.TxPipeline()
	pipe.Set(ctx, s.keys.script(cp.ID), payload, 0)
	pipe.SAdd(ctx, s.keys.index(), cp.ID)
	pipe.Incr(ctx, s.keys.gen())
	if _, err := pipe.Exec(ctx); err != nil {
		return errors.Wrap(err, "golua: 写入脚本缓存失败")
	}
	if s.invalidator != nil {
		s.invalidator.InvalidateScripts(cp.ID)
	}
	return nil
}
