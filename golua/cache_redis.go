// Package golua Redis 脚本缓存（写路径经 distlock 互斥）
//
// @author zwei
package golua

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/distlock"
	"gitee.com/hq_project/yomi-boot/errors"

	redisv9 "github.com/redis/go-redis/v9"
)

const (
	defaultRedisKeyPrefix = "yomi:golua"
	defaultRedisLockTTL   = 30 * time.Second
)

var _ scriptCache = (*redisScriptCache)(nil)

// redisScriptCache 基于 Redis 的脚本快照缓存（多实例共享；写操作持分布式锁）
type redisScriptCache struct {
	rdb     *redisv9.Client
	keys    redisKeys
	factory distlock.DistLockFactory
	lockTTL time.Duration
}

// newRedisScriptCache 创建 Redis 脚本缓存。
// keyPrefix 空则 yomi:golua；factory 必填，用于写路径跨实例互斥。
func newRedisScriptCache(rdb *redisv9.Client, keyPrefix string, factory distlock.DistLockFactory) (*redisScriptCache, error) {
	if rdb == nil {
		return nil, errors.New("golua: redis client 不能为空")
	}
	if factory == nil {
		return nil, errors.New("golua: DistLockFactory 不能为空")
	}
	prefix := strings.TrimSpace(keyPrefix)
	if prefix == "" {
		prefix = defaultRedisKeyPrefix
	}
	return &redisScriptCache{
		rdb:     rdb,
		keys:    newRedisKeys(prefix),
		factory: factory,
		lockTTL: defaultRedisLockTTL,
	}, nil
}

// Get 按 ID 读取（不加锁）
func (c *redisScriptCache) Get(ctx context.Context, id string) (*Script, error) {
	script, _, _, err := c.GetWithGen(ctx, id)
	return script, err
}

// GetWithGen 同轮 pipeline 读取脚本与 current_gen
func (c *redisScriptCache) GetWithGen(ctx context.Context, id string) (*Script, uint64, bool, error) {
	pipe := c.rdb.Pipeline()
	scriptCmd := pipe.Get(ctx, c.keys.script(id))
	genCmd := pipe.Get(ctx, c.keys.gen())
	_, pipeErr := pipe.Exec(ctx)
	_ = pipeErr // 单命令失败体现在各 Cmd

	raw, err := scriptCmd.Bytes()
	if err != nil {
		if errors.Is(err, redisv9.Nil) {
			return nil, 0, false, errors.Sentinel(ErrNotFound)
		}
		return nil, 0, false, errors.Wrap(err, "golua: 读取脚本缓存失败")
	}
	var script Script
	if err := json.Unmarshal(raw, &script); err != nil {
		return nil, 0, false, errors.Wrap(err, "golua: 解析脚本缓存失败")
	}
	if err := script.normalize(); err != nil {
		return nil, 0, false, err
	}

	gen, genOK := parseRedisGen(genCmd)
	return &script, gen, genOK, nil
}

// CurrentGen 读取共享代际
func (c *redisScriptCache) CurrentGen(ctx context.Context) (uint64, bool, error) {
	cmd := c.rdb.Get(ctx, c.keys.gen())
	if err := cmd.Err(); err != nil {
		if errors.Is(err, redisv9.Nil) {
			return 0, true, nil
		}
		return 0, false, errors.Wrap(err, "golua: 读取 current_gen 失败")
	}
	gen, genOK := parseRedisGen(cmd)
	if !genOK {
		return 0, false, errors.New("golua: current_gen 非法")
	}
	return gen, true, nil
}

// parseRedisGen 解析 gen 命令；key 不存在视为 0 且可用；解析失败 genOK=false
func parseRedisGen(cmd *redisv9.StringCmd) (gen uint64, genOK bool) {
	if cmd == nil {
		return 0, false
	}
	err := cmd.Err()
	if err != nil {
		if errors.Is(err, redisv9.Nil) {
			return 0, true
		}
		return 0, false
	}
	n, err := cmd.Uint64()
	if err != nil {
		return 0, false
	}
	return n, true
}

// Has 判断是否存在（不加锁）
func (c *redisScriptCache) Has(ctx context.Context, id string) (bool, error) {
	n, err := c.rdb.Exists(ctx, c.keys.script(id)).Result()
	if err != nil {
		return false, errors.Wrap(err, "golua: 检查脚本缓存失败")
	}
	return n > 0, nil
}

// Ready current_gen 存在且 index 非空（不加锁）
func (c *redisScriptCache) Ready(ctx context.Context) (bool, error) {
	gen, err := c.rdb.Exists(ctx, c.keys.gen()).Result()
	if err != nil {
		return false, errors.Wrap(err, "golua: 检查 current_gen 失败")
	}
	if gen == 0 {
		return false, nil
	}
	n, err := c.rdb.SCard(ctx, c.keys.index()).Result()
	if err != nil {
		return false, errors.Wrap(err, "golua: 检查 index 失败")
	}
	return n > 0, nil
}

// Put 写入单脚本（持锁）
func (c *redisScriptCache) Put(ctx context.Context, script *Script) error {
	return c.withWriteLock(ctx, func(ctx context.Context) error {
		return c.putLocked(ctx, script)
	})
}

// Remove 删除单脚本（持锁）
func (c *redisScriptCache) Remove(ctx context.Context, id string) error {
	return c.withWriteLock(ctx, func(ctx context.Context) error {
		return c.removeLocked(ctx, id)
	})
}

// ReplaceAll 全量替换并 reconcile（持锁）
func (c *redisScriptCache) ReplaceAll(ctx context.Context, scripts []*Script) error {
	return c.withWriteLock(ctx, func(ctx context.Context) error {
		return c.replaceAllLocked(ctx, scripts)
	})
}

// ReplaceAllFrom 持锁回源再全量替换，与 Load 共用同一把写锁
func (c *redisScriptCache) ReplaceAllFrom(ctx context.Context, load func(context.Context) ([]*Script, error)) error {
	if load == nil {
		return errors.New("golua: load 不能为空")
	}
	return c.withWriteLock(ctx, func(ctx context.Context) error {
		scripts, err := load(ctx)
		if err != nil {
			return err
		}
		return c.replaceAllLocked(ctx, scripts)
	})
}

// UpsertFrom 持锁回源单脚本：成功则 Put；NotFound 且 removeOnNotFound 则 Remove
func (c *redisScriptCache) UpsertFrom(ctx context.Context, id string, removeOnNotFound bool, load func(context.Context) (*Script, error)) error {
	if load == nil {
		return errors.New("golua: load 不能为空")
	}
	return c.withWriteLock(ctx, func(ctx context.Context) error {
		script, err := load(ctx)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				if !removeOnNotFound {
					return errors.Sentinel(ErrNotFound)
				}
				return c.removeLocked(ctx, id)
			}
			return err
		}
		if script == nil {
			return errors.New("golua: load 返回空脚本")
		}
		return c.putLocked(ctx, script)
	})
}

// withWriteLock 获取写锁后执行 fn（启用看门狗，避免长 ReplaceAll 丢锁）
func (c *redisScriptCache) withWriteLock(ctx context.Context, fn func(context.Context) error) error {
	lock := c.factory.GetLock(
		c.keys.lock(),
		distlock.WithTTL(c.lockTTL),
		distlock.WithWatchdog(true),
	)
	entered := false
	err := distlock.RunWithLock(ctx, lock, func(ctx context.Context) error {
		entered = true
		return fn(ctx)
	})
	if err == nil {
		return nil
	}
	// 未进入临界区 = 加锁失败；锁丢失 = 互斥失效；其余为业务写错误，原样返回
	if !entered || errors.Is(err, distlock.ErrLockLost) {
		return errors.Wrap(err, "golua: 脚本缓存写锁失败")
	}
	return err
}

// putLocked 已持锁写入
func (c *redisScriptCache) putLocked(ctx context.Context, script *Script) error {
	if script == nil {
		return errors.New("golua: 脚本不能为空")
	}
	cp := *script
	if err := cp.normalize(); err != nil {
		return err
	}
	payload, err := json.Marshal(&cp)
	if err != nil {
		return errors.Wrap(err, "golua: 序列化脚本失败")
	}
	pipe := c.rdb.TxPipeline()
	pipe.Set(ctx, c.keys.script(cp.ID), payload, 0)
	pipe.SAdd(ctx, c.keys.index(), cp.ID)
	pipe.Incr(ctx, c.keys.gen())
	if _, err := pipe.Exec(ctx); err != nil {
		return errors.Wrap(err, "golua: 写入脚本缓存失败")
	}
	return nil
}

// removeLocked 已持锁删除
func (c *redisScriptCache) removeLocked(ctx context.Context, id string) error {
	pipe := c.rdb.TxPipeline()
	pipe.Del(ctx, c.keys.script(id))
	pipe.SRem(ctx, c.keys.index(), id)
	pipe.Incr(ctx, c.keys.gen())
	if _, err := pipe.Exec(ctx); err != nil {
		return errors.Wrap(err, "golua: 删除脚本缓存失败")
	}
	return nil
}

// replaceAllLocked 已持锁全量替换
func (c *redisScriptCache) replaceAllLocked(ctx context.Context, scripts []*Script) error {
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

	oldIDs, err := c.rdb.SMembers(ctx, c.keys.index()).Result()
	if err != nil && !errors.Is(err, redisv9.Nil) {
		return errors.Wrap(err, "golua: 读取旧 index 失败")
	}

	pipe := c.rdb.TxPipeline()
	newIDs := make([]any, 0, len(normalized))
	for _, sc := range normalized {
		payload, err := json.Marshal(sc)
		if err != nil {
			return errors.Wrap(err, "golua: 序列化脚本失败")
		}
		pipe.Set(ctx, c.keys.script(sc.ID), payload, 0)
		newIDs = append(newIDs, sc.ID)
	}
	for _, old := range oldIDs {
		if _, ok := idSet[old]; !ok {
			pipe.Del(ctx, c.keys.script(old))
			pipe.SRem(ctx, c.keys.index(), old)
		}
	}
	if len(newIDs) > 0 {
		pipe.SAdd(ctx, c.keys.index(), newIDs...)
	}
	pipe.Incr(ctx, c.keys.gen())
	if _, err := pipe.Exec(ctx); err != nil {
		return errors.Wrap(err, "golua: 写入脚本缓存失败")
	}
	return nil
}
