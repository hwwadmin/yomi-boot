// Package idempotency 提供业务幂等守卫能力
//
// 以 sys_biz 幂等表为真源、Redis 结果缓存为快路径，配合分布式锁串行同键请求：
//   - 快路径：命中 Redis 结果缓存直接返回
//   - 分布式锁：缓存 miss 后串行化，锁内 double-check 缓存
//   - DB 查证：锁内查 sys_biz，已完成请求直接回放历史快照（纯读，不开启事务），并回写缓存自愈
//   - 事务抢占：INSERT 以「冲突即空操作」语义抢占处理权，执行业务并标记完成后提交，再回写缓存
//
// 双入口按事务形态区分：
//   - Execute：调用方 ctx 未绑定事务（最外层形态），事务提交后回写缓存快路径
//   - ExecuteInTx：调用方 ctx 已绑定同源事务（嵌套复用形态），抢占与完成标记跟随外层事务，
//     不写缓存——嵌套形态没有提交点，回写会跑在真实提交之前，外层回滚会留下假成功缓存
//
// 缓存回写失败只告警不影响业务结果，后续重复请求回源 DB 自愈。
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
	// claimMaxAttempts 是处理权抢占的总尝试次数上限：冲突且反查无记录时重抢一次，
	// 吸收「冲突的抢占行刚被回滚」的时序（嵌套形态下真实可见）
	claimMaxAttempts = 2
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

// snapshot 是已完成请求的可回放快照：result 返回给调用方，data 与 DB 存储的原始字节一致，
// 回写结果缓存时原样使用，保证 Redis 与 DB 的快照始终相同
type snapshot[T any] struct {
	result T
	data   []byte
}

// Execute 执行幂等保护（最外层事务形态），编排顺序：Redis 快路径 → 分布式锁 → 锁内 double-check →
// DB 查证回放 → 事务抢占执行 → 回写缓存
//
// T 必须与 fn 的返回类型一致且可 JSON 往返，DB/Redis 重放时会反序列化到同一类型。
// txRunner 由调用方注入，且必须是本次调用的最外层事务边界：传入的 ctx 已绑定同源事务时
// 返回 ErrNestedTx fail-fast——最外层形态的时序承诺（事务提交后才写缓存、抢占行对其他连接
// 可见）只有自己的提交点才能兑现；已处于事务内时改用 ExecuteInTx。
func Execute[T any](
	g *Guard,
	txRunner tx.Runner,
	ctx context.Context,
	key Key,
	fn func(ctx context.Context) (T, error),
) (T, error) {
	return execute[T](g, txRunner, ctx, key, fn, false)
}

// ExecuteInTx 执行幂等保护（嵌套复用形态）：调用方 ctx 已绑定同源事务时使用，
// 抢占、业务与完成标记全部跟随外层事务提交，本函数没有自己的提交点。
//
// 与 Execute 的编排完全一致，唯一差异是不写结果缓存：嵌套形态「Run 返回 ≠ 提交」，
// 回写缓存会跑在真实提交之前，外层回滚后会留下宣告成功而实际回滚的假成功缓存；
// 快路径由后续请求 DB 查证回放并自愈重建。语义边界（相对 Execute 的让步，均随外层事务）：
//   - 外层回滚则幂等记录随之消失，重试重新执行业务——与 Execute 失败重试的语义一致；
//     fn 的非事务性副作用（RPC、发消息）同样会重复，这不是嵌套形态引入的风险。
//   - MySQL REPEATABLE READ 下，并发方在己方事务快照建立后才提交完成记录时，冲突回退的
//     反查不可见，最终返回 ErrDuplicateWithoutRecord（fail-closed，下次请求回放自愈）。
//
// DTM 分支回调（barrier ctx）不适用本守卫：分支提交 ≠ 全局提交，分支幂等由 sys_barrier
// 承担，barrier ctx 的 InTx 为 false，会被下方校验以 ErrNotInTx 拒绝。
//
// 传入的 ctx 未绑定同源事务时返回 ErrNotInTx fail-fast：无外层事务时用 Execute，
// 它有自己的提交点，能提供缓存快路径。
func ExecuteInTx[T any](
	g *Guard,
	txRunner tx.Runner,
	ctx context.Context,
	key Key,
	fn func(ctx context.Context) (T, error),
) (T, error) {
	return execute[T](g, txRunner, ctx, key, fn, true)
}

// execute 是 Execute / ExecuteInTx 的共享编排；nested 标记嵌套复用形态（跟随外层事务、不写缓存）
func execute[T any](
	g *Guard,
	txRunner tx.Runner,
	ctx context.Context,
	key Key,
	fn func(ctx context.Context) (T, error),
	nested bool,
) (T, error) {
	var zero T

	// 校验事务运行器（参数校验）与幂等键
	if txRunner == nil {
		return zero, errors.Sentinel(ErrGuardNotReady)
	}
	// 两个入口按事务形态 fail-fast，防误用：Execute 拒绝嵌套（没有提交点），
	// ExecuteInTx 拒绝裸 ctx（没有外层事务可跟随）
	if !nested && txRunner.InTx(ctx) {
		return zero, errors.Sentinel(ErrNestedTx)
	}
	if nested && !txRunner.InTx(ctx) {
		return zero, errors.Sentinel(ErrNotInTx)
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
	lock := g.distLockFactory.GetLock(lockRedisKey(key), distlock.WithWatchdog(true))

	var result T
	err := distlock.RunWithLock(ctx, lock, func(runCtx context.Context) error {
		// Step 3: 锁内 double-check，避免并发 miss 重复走后续流程
		if cached, found, err := loadFromRedis[T](g, runCtx, rk); err != nil {
			return err
		} else if found {
			result = cached
			return nil
		}

		// Step 4: DB 查证——已完成请求直接回放历史快照（纯读，不开启事务），并回写缓存自愈快路径；
		// 嵌套形态不回写：反查到的可能是本事务未提交的行，回写会跑在真实提交之前
		snap, err := findCompletedSnapshot[T](g, runCtx, key)
		if err != nil {
			return err
		}
		if snap != nil {
			result = snap.result
			if !nested {
				writeCache(g, runCtx, rk, snap.data)
			}
			return nil
		}

		// Step 5: 事务内抢占处理权、执行业务并标记完成，任一环节失败回滚事务
		// （嵌套形态复用外层事务，回滚由最外层执行）
		if err := txRunner.Run(runCtx, func(txCtx context.Context) error {
			var innerErr error
			snap, innerErr = runBusinessOnce[T](g, txCtx, key, fn)
			return innerErr
		}); err != nil {
			return err
		}
		result = snap.result

		// Step 6: 非嵌套形态事务已提交，回写缓存供后续重复请求读取；
		// 嵌套形态没有提交点，不回写（防外层回滚后留下假成功缓存）
		if !nested {
			writeCache(g, runCtx, rk, snap.data)
		}
		return nil
	})
	if err != nil {
		return zero, err
	}
	return result, nil
}

// runBusinessOnce 在事务内执行一次业务：INSERT 抢占 → fn → MarkCompleted
//
// INSERT 冲突说明处理权已被他人拿走（DB 查证与抢占之间的窗口、锁丢失等），
// 从 DB 回放历史快照兜底；反查无记录说明抢占行刚被回滚（嵌套形态下真实可见），
// 有限重试抢占（上限 claimMaxAttempts）——全程 fail-closed，绝不重复执行业务。
func runBusinessOnce[T any](
	g *Guard,
	ctx context.Context,
	key Key,
	fn func(ctx context.Context) (T, error),
) (*snapshot[T], error) {
	id, err := g.idGenerator.NextID()
	if err != nil {
		return nil, err
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
	claimed, err := g.repo.TryInsert(ctx, row)
	if err != nil {
		return nil, err
	}
	for attempt := 1; !claimed; attempt++ {
		snap, findErr := findCompletedSnapshot[T](g, ctx, key)
		if findErr != nil {
			return nil, findErr
		}
		if snap != nil {
			return snap, nil
		}
		if attempt >= claimMaxAttempts {
			return nil, errors.Sentinel(ErrDuplicateWithoutRecord)
		}
		if claimed, err = g.repo.TryInsert(ctx, row); err != nil {
			return nil, err
		}
	}

	// 当前请求获得处理权，执行业务函数
	fnResult, err := fn(ctx)
	if err != nil {
		return nil, err
	}

	// 持久化响应快照并标记为已完成；快照字节同时用于回写结果缓存
	responseData, err := marshalResult(fnResult)
	if err != nil {
		return nil, err
	}
	if err := g.repo.MarkCompleted(ctx, row.ID, responseData); err != nil {
		return nil, err
	}
	return &snapshot[T]{result: fnResult, data: responseData}, nil
}

// findCompletedSnapshot 查询已完成请求的可回放快照，无已完成记录时返回 nil
//
// 处理中的记录属防御性场景（INSERT 与标记完成同事务提交，正常编排不可见），返回 ErrProcessing。
// 由 DB 查证与 INSERT 冲突回退两条路径共用，保证回放判定一致。
func findCompletedSnapshot[T any](g *Guard, ctx context.Context, key Key) (*snapshot[T], error) {
	row, err := g.repo.FindByKey(ctx, key.BizID, key.BizSubID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	if row.Status != StatusCompleted {
		return nil, errors.Sentinel(ErrProcessing)
	}
	result, err := unmarshalResult[T](row.ResponseData)
	if err != nil {
		return nil, err
	}
	return &snapshot[T]{result: result, data: row.ResponseData}, nil
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

// writeCache 将响应快照字节写入结果缓存；失败仅告警——缓存只是快路径，
// 后续重复请求会回源 DB 自愈，不允许影响业务结果
func writeCache(g *Guard, ctx context.Context, rk string, data []byte) {
	if err := g.redisService.OpsStr4Set(ctx, rk, string(data), defaultCacheTTL); err != nil {
		logger.Warn(ctx, "[幂等]结果缓存写入失败，后续重复请求将回源 DB", "redisKey", rk, "error", err)
	}
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
