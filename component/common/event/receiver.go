// Package event 提供收件箱幂等消费能力
//
// @author zwei
package event

import (
	"context"
	"time"

	"github.com/hwwadmin/yomi-boot/component/common/db/tx"
	"github.com/hwwadmin/yomi-boot/component/common/distlock"
	"github.com/hwwadmin/yomi-boot/component/common/event/cfg"
	"github.com/hwwadmin/yomi-boot/component/common/event/model"
	"github.com/hwwadmin/yomi-boot/component/common/event/repo"
	yomiredis "github.com/hwwadmin/yomi-boot/component/common/redis"
	"github.com/hwwadmin/yomi-boot/component/common/snowflake"
	"github.com/hwwadmin/yomi-boot/component/core/appctx"
	"github.com/hwwadmin/yomi-boot/component/core/errors"
	"github.com/hwwadmin/yomi-boot/component/core/logger"

	yomikafka "github.com/hwwadmin/yomi-boot/component/common/kafka"
)

// Receiver 幂等消息队列事件接收器；消费侧按 (ServiceName, EventID) 去重，要求发布侧 EventID 全局唯一
type Receiver struct {
	inboxRepo    *repo.InboxRepo
	inboxDlqRepo *repo.InboxDlqRepo
	txExecutor   tx.Executor
	lockFactory  distlock.DistLockFactory
	redis        yomiredis.RedisService
	cfg          cfg.Runtime
	idGen        snowflake.Generator
}

// NewReceiver 创建幂等事件接收器
func NewReceiver(
	inboxRepo *repo.InboxRepo,
	inboxDlqRepo *repo.InboxDlqRepo,
	txExecutor tx.Executor,
	lockFactory distlock.DistLockFactory,
	redisService yomiredis.RedisService,
	runtime cfg.Runtime,
	idGen snowflake.Generator,
) *Receiver {
	return &Receiver{
		inboxRepo:    inboxRepo,
		inboxDlqRepo: inboxDlqRepo,
		txExecutor:   txExecutor,
		lockFactory:  lockFactory,
		redis:        redisService,
		cfg:          runtime,
		idGen:        idGen,
	}
}

// receiveResult 消息接收处理结果
type receiveResult int

// 消息接收结果枚举
const (
	receiveSuccess receiveResult = iota
	receiveDuplicate
	receiveFail
)

// consumeLockUnlockTimeout 释放消费锁时调用 Redis 的最长等待，避免 Redis 不可用时永久阻塞（非持锁时长）
const consumeLockUnlockTimeout = 5 * time.Second

// inboxDlqInsertTimeout 写入收件箱死信的最长等待
const inboxDlqInsertTimeout = 5 * time.Second

// inboxDlqErrorMsgLimit 是收件箱死信错误信息的最大字符数，避免超过 text 列上限
const inboxDlqErrorMsgLimit = 4000

// inboxRetryCountTTL 是单条消息失败次数在 Redis 中的存活时间，每次失败都会刷新。
// 分区被抢走或进程重启后计数仍在；消息被确认后会主动删除，删除失败也靠这段时间回收。
const inboxRetryCountTTL = 7 * 24 * time.Hour

// luaInboxRetryIncr 原子累加失败次数并刷新过期时间。
// KEYS[1] = 计数键，ARGV[1] = 过期时间（毫秒），返回累加后的尝试次数。
const luaInboxRetryIncr = `
local n = redis.call('INCR', KEYS[1])
redis.call('PEXPIRE', KEYS[1], tonumber(ARGV[1]))
return n
`

// WrapHandler 将业务处理函数包装为 Kafka 消费 handler。
// 业务失败次数记在 Redis，跨进程累加；达到 InboxDlqMaxRetry 后写入 t_sys_event_inbox_dlq 并 ack。
// Kafka 的 MaxHandlerRetries 必须为 -1 或不小于该值，否则 Kafka 会先丢弃消息。
func (r *Receiver) WrapHandler(fn func(ctx context.Context, evt Event) error) yomikafka.ConsumeHandler {
	if fn == nil {
		return func(ctx context.Context, msg yomikafka.Message) yomikafka.ConsumeResult {
			logger.Warn(ctx, "[消息队列事件]action 未配置")
			return yomikafka.Retry()
		}
	}

	trackRetries := r.trackInboxRetries()
	return func(ctx context.Context, msg yomikafka.Message) yomikafka.ConsumeResult {
		if len(msg.Value) == 0 {
			logger.Warn(ctx, "[消息队列事件]消息为空，已被抛弃")
			return yomikafka.AckDrop()
		}

		evt, ok := ParseAndValidate(msg.Value, true)
		if !ok {
			return yomikafka.AckDrop()
		}

		// 启用看门狗续期（默认 TTL 30s），覆盖长链路业务
		lock := r.lockFactory.GetLock(r.cfg.LockKey(evt.EventID), distlock.WithWatchdog(true))
		held, err := lock.TryLock(ctx)
		if err != nil {
			logger.Error(ctx, "[消息队列事件]消费锁获取失败",
				"eventId", evt.EventID, "topic", evt.Topic, "error", err)
			return yomikafka.Retry()
		}
		if !held {
			exists, existErr := r.inboxRepo.Exists(ctx, r.cfg.ServiceName, evt.EventID)
			if existErr != nil {
				logger.Error(ctx, "[消息队列事件]重复校验失败",
					"eventId", evt.EventID, "topic", evt.Topic, "error", existErr)
				return yomikafka.Retry()
			}
			if exists {
				if trackRetries {
					r.resetInboxRetry(ctx, msg)
				}
				logger.Warn(ctx, "[消息队列事件]重复且已入 inbox",
					"eventId", evt.EventID, "topic", evt.Topic)
				return yomikafka.AckOK()
			}
			logger.Warn(ctx, "[消息队列事件]正在处理中，暂不 ack",
				"eventId", evt.EventID, "topic", evt.Topic)
			return yomikafka.Retry()
		}

		defer func() {
			// 使用独立超时 ctx，避免消费 ctx 已取消导致 Unlock 失败、看门狗未停
			unlockCtx, cancel := appctx.BackgroundWithTimeout(consumeLockUnlockTimeout)
			defer cancel()
			if unlockErr := lock.Unlock(unlockCtx); unlockErr != nil && !errors.Is(unlockErr, distlock.ErrLockNotHeld) {
				logger.Error(ctx, "[消息队列事件]消费锁释放失败",
					"eventId", evt.EventID, "topic", evt.Topic, "error", unlockErr)
			}
		}()

		// 锁丢失时 cancel runCtx，让事务与业务尽快退出，缩小并发双跑窗口
		runCtx, stop := distlock.BindHeldContext(ctx, lock)
		defer stop()

		result := receiveFail
		txErr := r.txExecutor.Run(runCtx, func(txCtx context.Context) error {
			exists, existErr := r.inboxRepo.Exists(txCtx, r.cfg.ServiceName, evt.EventID)
			if existErr != nil {
				return existErr
			}
			if exists {
				result = receiveDuplicate
				return nil
			}

			inboxID, idErr := r.idGen.NextID()
			if idErr != nil {
				return idErr
			}
			inbox := &model.SysEventInbox{
				ID:          inboxID,
				ServiceName: r.cfg.ServiceName,
				EventID:     evt.EventID,
				Topic:       evt.Topic,
				Payload:     evt.Payload,
			}
			if err := r.inboxRepo.Insert(txCtx, inbox); err != nil {
				return err
			}
			if err := invokeEventHandler(fn, txCtx, evt); err != nil {
				return err
			}
			result = receiveSuccess
			return nil
		})
		if txErr != nil {
			logger.Error(ctx, "[消息队列事件]接收失败",
				"eventId", evt.EventID, "topic", evt.Topic, "error", txErr)
			// 停机/锁丢失/下游超时属于可恢复的上下文错误，不参与死信计数
			if !trackRetries ||
				errors.Is(txErr, context.Canceled) ||
				errors.Is(txErr, context.DeadlineExceeded) {
				return yomikafka.Retry()
			}
			retryCount, countErr := r.incrementInboxRetry(ctx, msg)
			if countErr != nil {
				logger.Error(ctx, "[消息队列事件]累计重试次数失败",
					"eventId", evt.EventID, "topic", evt.Topic, "error", countErr)
				return yomikafka.Retry()
			}
			if !r.shouldMoveToInboxDlq(retryCount) {
				return yomikafka.Retry()
			}
			if dlqErr := r.moveToInboxDlq(ctx, evt, txErr, retryCount); dlqErr != nil {
				logger.Error(ctx, "[消息队列事件]转入收件箱死信失败",
					"eventId", evt.EventID, "topic", evt.Topic,
					"retryCount", retryCount, "error", dlqErr)
				return yomikafka.Retry()
			}
			r.resetInboxRetry(ctx, msg)
			logger.Error(ctx, "[消息队列事件]重试次数已超限，已转入收件箱死信",
				"eventId", evt.EventID, "topic", evt.Topic,
				"retryCount", retryCount, "error", txErr)
			return yomikafka.AckOK()
		}

		switch result {
		case receiveSuccess:
			if trackRetries {
				r.resetInboxRetry(ctx, msg)
			}
			logger.Info(ctx, "[消息队列事件]接收成功", "eventId", evt.EventID, "topic", evt.Topic)
			return yomikafka.AckOK()
		case receiveDuplicate:
			if trackRetries {
				r.resetInboxRetry(ctx, msg)
			}
			logger.Warn(ctx, "[消息队列事件]重复", "eventId", evt.EventID, "topic", evt.Topic)
			return yomikafka.AckOK()
		default:
			return yomikafka.Retry()
		}
	}
}

// invokeEventHandler 调用业务处理函数并捕获 panic，转为可记账的普通错误
func invokeEventHandler(fn func(ctx context.Context, evt Event) error, ctx context.Context, evt Event) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Newf("事件处理 panic: %v", r)
		}
	}()
	return fn(ctx, evt)
}

// trackInboxRetries 返回是否需要累计业务失败次数。负数表示无限重试，不访问 Redis。
func (r *Receiver) trackInboxRetries() bool {
	return r.inboxDlqRepo != nil && r.cfg.InboxDlqMaxRetry >= 0
}

// shouldMoveToInboxDlq 判断当前失败次数是否已达到转入收件箱死信的条件
func (r *Receiver) shouldMoveToInboxDlq(retryCount int) bool {
	return r.trackInboxRetries() && retryCount >= r.cfg.InboxDlqMaxRetry
}

// incrementInboxRetry 累加该 Kafka 消息的失败次数，返回不含首次处理的重试次数。
// Redis 不可用时返回错误，调用方应继续重试，不能据此转入死信。
func (r *Receiver) incrementInboxRetry(ctx context.Context, msg yomikafka.Message) (int, error) {
	if r.redis == nil {
		return 0, errors.New("event: Redis 服务未初始化")
	}
	result, err := r.redis.OpsScript4Eval(
		ctx,
		luaInboxRetryIncr,
		[]string{r.cfg.RetryCountKey(msg.Topic, msg.Partition, msg.Offset)},
		inboxRetryCountTTL.Milliseconds(),
	)
	if err != nil {
		return 0, err
	}
	attempts, err := redisInteger(result)
	if err != nil {
		return 0, err
	}
	return attempts - 1, nil
}

// resetInboxRetry 删除该 Kafka 消息的失败次数。删除失败只记日志，键会按 TTL 过期。
func (r *Receiver) resetInboxRetry(ctx context.Context, msg yomikafka.Message) {
	if r.redis == nil {
		return
	}
	key := r.cfg.RetryCountKey(msg.Topic, msg.Partition, msg.Offset)
	if err := r.redis.OpsKey4Delete(ctx, key); err != nil {
		logger.Error(ctx, "[消息队列事件]清除重试计数失败",
			"topic", msg.Topic, "partition", msg.Partition, "offset", msg.Offset, "error", err)
	}
}

// redisInteger 解析 Lua 数值返回值
func redisInteger(result any) (int, error) {
	switch value := result.(type) {
	case int64:
		return int(value), nil
	case int:
		return value, nil
	default:
		return 0, errors.Newf("event: 重试计数返回类型异常: %T", result)
	}
}

// moveToInboxDlq 将超过重试上限的事件写入收件箱死信
func (r *Receiver) moveToInboxDlq(ctx context.Context, evt Event, cause error, retryCount int) error {
	if r.inboxDlqRepo == nil {
		return errors.New("event: 收件箱死信仓储未初始化")
	}
	id, err := r.idGen.NextID()
	if err != nil {
		return err
	}
	dlqCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), inboxDlqInsertTimeout)
	defer cancel()
	return r.inboxDlqRepo.Insert(dlqCtx, &model.SysEventInboxDlq{
		ID:          id,
		ServiceName: r.cfg.ServiceName,
		EventID:     evt.EventID,
		Topic:       evt.Topic,
		Payload:     evt.Payload,
		ErrorMsg:    truncateInboxDlqError(cause),
		RetryCount:  retryCount,
	})
}

// truncateInboxDlqError 截断收件箱死信错误信息，避免超过 text 列上限
func truncateInboxDlqError(err error) string {
	if err == nil {
		return ""
	}
	runes := []rune(err.Error())
	if len(runes) <= inboxDlqErrorMsgLimit {
		return string(runes)
	}
	return string(runes[:inboxDlqErrorMsgLimit]) + "...[truncated]"
}
