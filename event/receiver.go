// Package event 提供收件箱幂等消费能力
//
// @author zwei
package event

import (
	"context"
	"time"
	"yt-boot/appctx"
	"yt-boot/db"
	"yt-boot/distlock"
	"yt-boot/errors"
	"yt-boot/event/cfg"
	"yt-boot/event/model"
	"yt-boot/event/repo"
	"yt-boot/logger"
	"yt-boot/snowflake"

	ytkafka "yt-boot/kafka"
)

// Receiver 幂等消息队列事件接收器
type Receiver struct {
	inboxRepo   *repo.InboxRepo
	txExecutor  db.TxExecutor
	lockFactory distlock.DistLockFactory
	cfg         cfg.Runtime
	idGen       snowflake.Generator
}

// NewReceiver 创建幂等事件接收器
func NewReceiver(
	inboxRepo *repo.InboxRepo,
	txExecutor db.TxExecutor,
	lockFactory distlock.DistLockFactory,
	runtime cfg.Runtime,
	idGen snowflake.Generator,
) *Receiver {
	return &Receiver{
		inboxRepo:   inboxRepo,
		txExecutor:  txExecutor,
		lockFactory: lockFactory,
		cfg:         runtime,
		idGen:       idGen,
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

// WrapHandler 将业务处理函数包装为 Kafka 消费 handler
func (r *Receiver) WrapHandler(fn func(ctx context.Context, evt Event) error) ytkafka.ConsumeHandler {
	if fn == nil {
		return func(ctx context.Context, msg ytkafka.Message) ytkafka.ConsumeResult {
			logger.Warn(ctx, "消息队列事件 action 未配置")
			return ytkafka.Retry()
		}
	}

	return func(ctx context.Context, msg ytkafka.Message) ytkafka.ConsumeResult {
		if len(msg.Value) == 0 {
			logger.Warn(ctx, "消息队列事件为空，已被抛弃")
			return ytkafka.AckDrop()
		}

		evt, ok := ParseAndValidate(msg.Value, true)
		if !ok {
			return ytkafka.AckDrop()
		}

		// 启用看门狗续期（默认 TTL 30s），覆盖长链路业务
		lock := r.lockFactory.GetLock(r.cfg.LockKey(evt.EventID), distlock.WithWatchdog(true))
		held, err := lock.TryLock(ctx)
		if err != nil {
			logger.Error(ctx, "消息队列事件消费锁获取失败",
				"eventId", evt.EventID, "topic", evt.Topic, "error", err)
			return ytkafka.Retry()
		}
		if !held {
			exists, existErr := r.inboxRepo.Exists(ctx, r.cfg.ServiceName, evt.EventID)
			if existErr != nil {
				logger.Error(ctx, "消息队列事件重复校验失败",
					"eventId", evt.EventID, "topic", evt.Topic, "error", existErr)
				return ytkafka.Retry()
			}
			if exists {
				logger.Warn(ctx, "消息队列事件重复且已入 inbox",
					"eventId", evt.EventID, "topic", evt.Topic)
				return ytkafka.AckOK()
			}
			logger.Warn(ctx, "消息队列事件正在处理中，暂不 ack",
				"eventId", evt.EventID, "topic", evt.Topic)
			return ytkafka.Retry()
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
		txErr := r.txExecutor.Execute(runCtx, func(txCtx context.Context) error {
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
				BaseModel: db.BaseModel{
					ID: inboxID,
				},
			}
			if err := r.inboxRepo.Insert(txCtx, inbox); err != nil {
				return err
			}
			if err := fn(txCtx, evt); err != nil {
				return err
			}
			result = receiveSuccess
			return nil
		})
		if txErr != nil {
			logger.Error(ctx, "消息队列事件接收失败",
				"eventId", evt.EventID, "topic", evt.Topic, "error", txErr)
			return ytkafka.Retry()
		}

		switch result {
		case receiveSuccess:
			logger.Info(ctx, "消息队列事件接收成功", "eventId", evt.EventID, "topic", evt.Topic)
			return ytkafka.AckOK()
		case receiveDuplicate:
			logger.Warn(ctx, "消息队列事件重复", "eventId", evt.EventID, "topic", evt.Topic)
			return ytkafka.AckOK()
		default:
			return ytkafka.Retry()
		}
	}
}
