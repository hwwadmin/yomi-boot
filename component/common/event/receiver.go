// Package event 提供收件箱幂等消费能力
//
// @author zwei
package event

import (
	"context"
	"sync"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/db/orm"
	"gitee.com/hq_project/yomi-boot/component/common/db/tx"
	"gitee.com/hq_project/yomi-boot/component/common/distlock"
	"gitee.com/hq_project/yomi-boot/component/common/event/cfg"
	"gitee.com/hq_project/yomi-boot/component/common/event/model"
	"gitee.com/hq_project/yomi-boot/component/common/event/repo"
	"gitee.com/hq_project/yomi-boot/component/common/snowflake"
	"gitee.com/hq_project/yomi-boot/component/core/appctx"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/logger"

	yomikafka "gitee.com/hq_project/yomi-boot/component/common/kafka"
)

// Receiver 幂等消息队列事件接收器；消费侧按 (ServiceName, EventID) 去重，要求发布侧 EventID 全局唯一
type Receiver struct {
	inboxRepo    *repo.InboxRepo
	inboxDlqRepo *repo.InboxDlqRepo
	txExecutor   tx.Executor
	lockFactory  distlock.DistLockFactory
	cfg          cfg.Runtime
	idGen        snowflake.Generator
}

// NewReceiver 创建幂等事件接收器
func NewReceiver(
	inboxRepo *repo.InboxRepo,
	inboxDlqRepo *repo.InboxDlqRepo,
	txExecutor tx.Executor,
	lockFactory distlock.DistLockFactory,
	runtime cfg.Runtime,
	idGen snowflake.Generator,
) *Receiver {
	return &Receiver{
		inboxRepo:    inboxRepo,
		inboxDlqRepo: inboxDlqRepo,
		txExecutor:   txExecutor,
		lockFactory:  lockFactory,
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

// WrapHandler 将业务处理函数包装为 Kafka 消费 handler。
// 业务失败达到 InboxDlqMaxRetry 后写入 sys_event_inbox_dlq 并 ack；
// Kafka 的 MaxHandlerRetries 必须为 -1 或不小于该值，否则 Kafka 会先丢弃消息。
func (r *Receiver) WrapHandler(fn func(ctx context.Context, evt Event) error) yomikafka.ConsumeHandler {
	if fn == nil {
		return func(ctx context.Context, msg yomikafka.Message) yomikafka.ConsumeResult {
			logger.Warn(ctx, "消息队列事件 action 未配置")
			return yomikafka.Retry()
		}
	}

	trackRetries := r.inboxDlqRepo != nil && r.cfg.InboxDlqMaxRetry >= 0
	retryTracker := newMessageRetryTracker()
	return func(ctx context.Context, msg yomikafka.Message) yomikafka.ConsumeResult {
		if len(msg.Value) == 0 {
			logger.Warn(ctx, "消息队列事件为空，已被抛弃")
			return yomikafka.AckDrop()
		}

		evt, ok := ParseAndValidate(msg.Value, true)
		if !ok {
			return yomikafka.AckDrop()
		}
		retryKey := messageRetryKey{topic: msg.Topic, partition: msg.Partition, offset: msg.Offset}

		// 启用看门狗续期（默认 TTL 30s），覆盖长链路业务
		lock := r.lockFactory.GetLock(r.cfg.LockKey(evt.EventID), distlock.WithWatchdog(true))
		held, err := lock.TryLock(ctx)
		if err != nil {
			logger.Error(ctx, "消息队列事件消费锁获取失败",
				"eventId", evt.EventID, "topic", evt.Topic, "error", err)
			return yomikafka.Retry()
		}
		if !held {
			exists, existErr := r.inboxRepo.Exists(ctx, r.cfg.ServiceName, evt.EventID)
			if existErr != nil {
				logger.Error(ctx, "消息队列事件重复校验失败",
					"eventId", evt.EventID, "topic", evt.Topic, "error", existErr)
				return yomikafka.Retry()
			}
			if exists {
				retryTracker.reset(retryKey)
				logger.Warn(ctx, "消息队列事件重复且已入 inbox",
					"eventId", evt.EventID, "topic", evt.Topic)
				return yomikafka.AckOK()
			}
			logger.Warn(ctx, "消息队列事件正在处理中，暂不 ack",
				"eventId", evt.EventID, "topic", evt.Topic)
			return yomikafka.Retry()
		}

		defer func() {
			// 使用独立超时 ctx，避免消费 ctx 已取消导致 Unlock 失败、看门狗未停
			unlockCtx, cancel := appctx.BackgroundWithTimeout(consumeLockUnlockTimeout)
			defer cancel()
			if unlockErr := lock.Unlock(unlockCtx); unlockErr != nil && !errors.Is(unlockErr, distlock.ErrLockNotHeld) {
				logger.Error(ctx, "消息队列事件消费锁释放失败",
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
				ServiceName: r.cfg.ServiceName,
				EventID:     evt.EventID,
				Topic:       evt.Topic,
				Payload:     evt.Payload,
				BaseModel: orm.BaseModel{
					ID: inboxID,
				},
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
			logger.Error(ctx, "消息队列事件接收失败",
				"eventId", evt.EventID, "topic", evt.Topic, "error", txErr)
			// 停机/锁丢失/下游超时属于可恢复的上下文错误，不参与死信计数
			if !trackRetries ||
				errors.Is(txErr, context.Canceled) ||
				errors.Is(txErr, context.DeadlineExceeded) {
				return yomikafka.Retry()
			}
			retryCount := retryTracker.increment(retryKey) - 1
			if !r.shouldMoveToInboxDlq(retryCount) {
				return yomikafka.Retry()
			}
			if dlqErr := r.moveToInboxDlq(ctx, evt, txErr, retryCount); dlqErr != nil {
				logger.Error(ctx, "消息队列事件转入收件箱死信失败",
					"eventId", evt.EventID, "topic", evt.Topic,
					"retryCount", retryCount, "error", dlqErr)
				return yomikafka.Retry()
			}
			retryTracker.reset(retryKey)
			logger.Error(ctx, "消息队列事件重试次数已超限，已转入收件箱死信",
				"eventId", evt.EventID, "topic", evt.Topic,
				"retryCount", retryCount, "error", txErr)
			return yomikafka.AckOK()
		}

		switch result {
		case receiveSuccess:
			retryTracker.reset(retryKey)
			logger.Info(ctx, "消息队列事件接收成功", "eventId", evt.EventID, "topic", evt.Topic)
			return yomikafka.AckOK()
		case receiveDuplicate:
			retryTracker.reset(retryKey)
			logger.Warn(ctx, "消息队列事件重复", "eventId", evt.EventID, "topic", evt.Topic)
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

// shouldMoveToInboxDlq 判断当前失败次数是否已达到转入收件箱死信的条件
func (r *Receiver) shouldMoveToInboxDlq(retryCount int) bool {
	return r.inboxDlqRepo != nil && r.cfg.InboxDlqMaxRetry >= 0 && retryCount >= r.cfg.InboxDlqMaxRetry
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
		ServiceName: r.cfg.ServiceName,
		EventID:     evt.EventID,
		Topic:       evt.Topic,
		Payload:     evt.Payload,
		ErrorMsg:    truncateInboxDlqError(cause),
		RetryCount:  retryCount,
		BaseModel: orm.BaseModel{
			ID: id,
		},
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

// messageRetryKey 是同一条 Kafka 消息的重试计数键
type messageRetryKey struct {
	topic     string
	partition int
	offset    int64
}

// messageRetryTracker 记录同一条 Kafka 消息的失败重试次数
type messageRetryTracker struct {
	mu     sync.Mutex
	counts map[messageRetryKey]int
}

// newMessageRetryTracker 创建消息重试计数器
func newMessageRetryTracker() *messageRetryTracker {
	return &messageRetryTracker{counts: make(map[messageRetryKey]int)}
}

// increment 增加并返回指定消息的失败次数
func (t *messageRetryTracker) increment(key messageRetryKey) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.counts[key]++
	return t.counts[key]
}

// reset 清除指定消息的失败次数
func (t *messageRetryTracker) reset(key messageRetryKey) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.counts, key)
}
