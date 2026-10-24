// Package idempotency 提供业务幂等守卫能力
//
// 采用 Redis 结果缓存 + 分布式锁 + DB 幂等表三层策略：
//   - 第一层：Redis 结果缓存（热读，重复请求直接返回）
//   - 第二层：分布式锁（miss 后串行化，避免并发执行业务）
//   - 第三层：DB 唯一索引 + 幂等表（持久化兜底）
//
// @author zwei
package idempotency

import (
	"context"
	"encoding/json"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/db/orm"
	"gitee.com/hq_project/yomi-boot/component/common/db/tx"
	"gitee.com/hq_project/yomi-boot/component/common/distlock"
	yomiredis "gitee.com/hq_project/yomi-boot/component/common/redis"
	"gitee.com/hq_project/yomi-boot/component/common/snowflake"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/logger"

	"github.com/google/uuid"
)

const (
	defaultCacheTTL          = 5 * time.Minute     // defaultCacheTTL 是幂等结果在 Redis 中的默认过期时间
	resultKeyPrefix          = "sysbiz:result:"    // resultKeyPrefix 是结果缓存 Key 前缀
	idempotencyLockKeyPrefix = "idempotency:lock:" // idempotencyLockKeyPrefix 是幂等分布式锁 Key 前缀
)

// Guard 业务幂等守卫
type Guard struct {
	repo            *Repository
	idGenerator     snowflake.Generator
	redisService    yomiredis.RedisService
	distLockFactory distlock.DistLockFactory
}

// NewGuard 创建守卫
// repo、idGenerator、redisService、distLockFactory 为必填依赖，缺失时启动期 fail-fast
func NewGuard(
	repo *Repository,
	idGenerator snowflake.Generator,
	redisService yomiredis.RedisService,
	distLockFactory distlock.DistLockFactory,
) *Guard {
	if repo == nil {
		panic("idempotency: NewGuard repo 不能为 nil")
	}
	if idGenerator == nil {
		panic("idempotency: NewGuard idGenerator 不能为 nil")
	}
	if redisService == nil {
		panic("idempotency: NewGuard redisService 不能为 nil")
	}
	if distLockFactory == nil {
		panic("idempotency: NewGuard distLockFactory 不能为 nil")
	}
	return &Guard{
		repo:            repo,
		idGenerator:     idGenerator,
		redisService:    redisService,
		distLockFactory: distLockFactory,
	}
}

// Execute 执行幂等保护，按 Redis 快路径 → 分布式锁 → 事务 → commit 后写 Redis 的顺序编排
//
// T 必须与 fn 的返回类型一致，DB/Redis 重放时会反序列化到同一类型。
// txRunner 由调用方注入；本地事务可直接传 tx.Executor（内嵌 Runner），Saga 分支传 *dtm.TxRunner。
func Execute[T any](
	g *Guard,
	txRunner tx.Runner,
	ctx context.Context,
	key Key,
	fn func(ctx context.Context) (T, error),
) (T, error) {
	var zero T

	// 校验事务运行器（参数校验）与幂等键
	if txRunner == nil {
		return zero, errors.Sentinel(ErrGuardNotReady)
	}
	if key.BizID == "" || key.BizSubID == "" || key.OperationType == "" {
		return zero, errors.Sentinel(ErrKeyInvalid)
	}

	rk := resultRedisKey(key)

	// Step 1: 无锁快路径，命中结果缓存直接返回
	if result, found, err := loadFromRedis[T](g, ctx, rk); err != nil {
		return zero, err
	} else if found {
		return result, nil
	}

	// Step 2: 缓存未命中，按幂等键获取分布式锁并启用看门狗续期
	lock := g.distLockFactory.GetLock(
		lockRedisKey(key),
		distlock.WithWatchdog(true),
	)

	var result T
	err := distlock.RunWithLock(ctx, lock, func(runCtx context.Context) error {
		// Step 3: 锁内 double-check，避免并发 miss 重复执行业务
		if cached, found, err := loadFromRedis[T](g, runCtx, rk); err != nil {
			return err
		} else if found {
			result = cached
			return nil
		}

		// Step 4: 事务内 INSERT 抢处理权、执行业务并标记完成
		var bizResult T
		if err := txRunner.Run(runCtx, func(txCtx context.Context) error {
			var innerErr error
			bizResult, innerErr = runBusinessOnce(g, txCtx, key, fn)
			return innerErr
		}); err != nil {
			return err
		}

		// Step 5: 事务已 commit，写入结果缓存供后续重复请求读取
		responseData, err := marshalResult(bizResult)
		if err != nil {
			return err
		}
		if err := saveToRedis(g, runCtx, rk, responseData); err != nil {
			logger.Error(runCtx, "[幂等]结果缓存写入失败，业务已成功",
				"bizId", key.BizID, "bizSubId", key.BizSubID, "redisKey", rk, "error", err)
		}
		result = bizResult
		return nil
	})
	if err != nil {
		return zero, err
	}
	return result, nil
}

// runBusinessOnce 在事务内执行一次业务：INSERT 抢锁 → fn → MarkCompleted
//
// INSERT 唯一键冲突时不再执行业务，直接从 DB 读取已完成结果
func runBusinessOnce[T any](
	g *Guard,
	ctx context.Context,
	key Key,
	fn func(ctx context.Context) (T, error),
) (T, error) {
	var zero T

	id, err := g.idGenerator.NextID()
	if err != nil {
		return zero, err
	}
	row := &SysBiz{
		BizID:         key.BizID,
		BizSubID:      key.BizSubID,
		OperationType: key.OperationType,
		Status:        StatusProcessing,
		BaseModel: orm.BaseModel{
			ID: id,
		},
	}

	// 以「冲突即空操作」语义抢占处理权：冲突说明已有请求在处理或已完成，
	// 且插入不报错、事务不中止，可直接在当前事务内反查已完成结果
	claimed, insertErr := g.repo.TryInsert(ctx, row)
	if insertErr != nil {
		return zero, insertErr
	}
	if !claimed {
		return loadCompletedResult[T](g, ctx, key)
	}

	// 当前请求获得处理权，执行业务函数
	fnResult, fnErr := fn(ctx)
	if fnErr != nil {
		return zero, fnErr
	}

	// 持久化响应快照并标记为已完成
	responseData, fnErr := marshalResult(fnResult)
	if fnErr != nil {
		return zero, fnErr
	}
	if fnErr = g.repo.MarkCompleted(ctx, row.ID, responseData); fnErr != nil {
		return zero, fnErr
	}
	return fnResult, nil
}

// loadCompletedResult 从 DB 读取已完成幂等记录并反序列化为 T
//
// 记录不存在返回 ErrDuplicateWithoutRecord，仍在处理中返回 ErrProcessing
func loadCompletedResult[T any](g *Guard, ctx context.Context, key Key) (T, error) {
	var zero T

	existing, findErr := g.repo.FindByKey(ctx, key.BizID, key.BizSubID)
	if findErr != nil {
		return zero, findErr
	}
	if existing == nil {
		return zero, errors.Sentinel(ErrDuplicateWithoutRecord)
	}
	if existing.Status != StatusCompleted {
		return zero, errors.Sentinel(ErrProcessing)
	}
	return unmarshalResult[T](existing.ResponseData)
}

// loadFromRedis 从 Redis 读取并反序列化幂等结果
//
// found 为 false 表示 Key 不存在，err 非 nil 表示 Redis 读写失败
func loadFromRedis[T any](g *Guard, ctx context.Context, rk string) (T, bool, error) {
	var zero T
	payload, exists, err := g.redisService.OpsStr4Get(ctx, rk)
	if err != nil {
		return zero, false, err
	}
	if !exists {
		return zero, false, nil
	}
	result, err := unmarshalResult[T]([]byte(payload))
	if err != nil {
		return zero, false, err
	}
	return result, true, nil
}

// saveToRedis 将响应快照写入 Redis 结果缓存
func saveToRedis(g *Guard, ctx context.Context, rk string, responseData []byte) error {
	payload := string(responseData)
	if payload == "" {
		payload = "{}"
	}
	return g.redisService.OpsStr4Set(ctx, rk, payload, defaultCacheTTL)
}

// resultRedisKey 生成结果缓存 Redis Key
func resultRedisKey(key Key) string {
	return resultKeyPrefix + key.BizID + ":" + key.BizSubID
}

// lockRedisKey 生成幂等分布式锁 Redis Key
func lockRedisKey(key Key) string {
	return idempotencyLockKeyPrefix + key.BizID + ":" + key.BizSubID
}

// unmarshalResult 将 JSON 字节反序列化为 T，空数据返回 T 的零值
func unmarshalResult[T any](data []byte) (T, error) {
	var zero T
	if len(data) == 0 {
		return zero, nil
	}
	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		return zero, err
	}
	return result, nil
}

// marshalResult 将 T 序列化为 JSON 字节，null 结果返回 nil
func marshalResult[T any](value T) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if string(data) == "null" {
		return nil, nil
	}
	return data, nil
}

// BizID 生成新的业务幂等 ID，用于 BFF 出站写入
func BizID() string {
	return uuid.NewString()
}
