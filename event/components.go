// Package event 组装消息队列事件组件
//
// @author zwei
package event

import (
	"context"
	"io"
	"time"

	"gitee.com/hq_project/yomi-boot/db"
	"gitee.com/hq_project/yomi-boot/distlock"
	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/event/cfg"
	"gitee.com/hq_project/yomi-boot/event/outbox"
	"gitee.com/hq_project/yomi-boot/event/repo"
	"gitee.com/hq_project/yomi-boot/logger"
	"gitee.com/hq_project/yomi-boot/shared/enums"
	"gitee.com/hq_project/yomi-boot/snowflake"

	yomikafka "gitee.com/hq_project/yomi-boot/kafka"

	"gorm.io/gorm"
)

// Deps 事件组件依赖
type Deps struct {
	DB              *gorm.DB
	TxExecutor      db.TxExecutor
	KafkaPublisher  yomikafka.PublisherService
	DistLockFactory distlock.DistLockFactory
	IDGenerator     snowflake.Generator
	DbType          enums.DbType
	AppName         string
}

// Components 消息队列事件组件
type Components struct {
	Publisher      *Publisher
	Receiver       *Receiver
	NormalReceiver *NormalReceiver
	cfg            cfg.Runtime

	outboxRepo       *repo.OutboxRepo
	dlqHandler       *outbox.DlqHandler
	inboxDlqHandler  *InboxDlqHandler
	retentionCleaner *RetentionCleaner
	processor        *outbox.Processor
	kafkaPublisher   yomikafka.PublisherService
	txExecutor       db.TxExecutor
	lockFactory      distlock.DistLockFactory

	cancel  context.CancelFunc
	started bool
}

var _ io.Closer = (*Components)(nil)

// NewComponents 创建消息队列事件组件（按 publisher/consumer 开关装配）
func NewComponents(deps Deps, eventCfg Config) (*Components, error) {
	if !eventCfg.AnyEnabledOrDefault() {
		return nil, errors.New("event: 发布侧与消费侧均未启用")
	}
	if deps.DB == nil {
		return nil, errors.New("event: 数据库未初始化")
	}
	if deps.TxExecutor == nil {
		return nil, errors.New("event: 事务执行器未初始化")
	}
	if deps.IDGenerator == nil {
		return nil, errors.New("event: ID 生成器未初始化")
	}

	runtime := runtimeFrom(eventCfg, deps.AppName)
	c := &Components{
		cfg:         runtime,
		txExecutor:  deps.TxExecutor,
		lockFactory: deps.DistLockFactory,
	}
	outboxRepo := repo.NewOutboxRepo(deps.DB)
	inboxRepo := repo.NewInboxRepo(deps.DB)

	if runtime.PublisherEnabled {
		if deps.KafkaPublisher == nil {
			return nil, errors.New("event: Kafka 发布者未初始化")
		}
		dlqRepo := repo.NewOutboxDlqRepo(deps.DB)
		c.Publisher = NewPublisher(outboxRepo, runtime, deps.IDGenerator, deps.DbType)
		c.outboxRepo = outboxRepo
		c.dlqHandler = outbox.NewDlqHandler(
			runtime, outboxRepo, dlqRepo, deps.KafkaPublisher, deps.TxExecutor, deps.IDGenerator,
		)
		c.kafkaPublisher = deps.KafkaPublisher
	}

	if runtime.ConsumerEnabled {
		if deps.DistLockFactory == nil {
			return nil, errors.New("event: 分布式锁工厂未初始化")
		}
		inboxDlqRepo := repo.NewInboxDlqRepo(deps.DB)
		c.Receiver = NewReceiver(
			inboxRepo, inboxDlqRepo, deps.TxExecutor, deps.DistLockFactory, runtime, deps.IDGenerator,
		)
		c.inboxDlqHandler = NewInboxDlqHandler(runtime, inboxDlqRepo, inboxRepo, deps.KafkaPublisher)
		c.NormalReceiver = NewNormalReceiver()
	}

	if runtime.OutboxCleanup.Enabled || runtime.InboxCleanup.Enabled {
		c.retentionCleaner = NewRetentionCleaner(runtime, outboxRepo, inboxRepo)
	}

	logger.InfoWithoutCtx("[消息队列事件]主体服务名",
		"serviceName", runtime.ServiceName,
		"publisherEnabled", runtime.PublisherEnabled,
		"consumerEnabled", runtime.ConsumerEnabled,
	)
	return c, nil
}

// NewAndStart 创建并启动事件组件；任一步失败时在返回前释放已创建资源
func NewAndStart(deps Deps, eventCfg Config) (c *Components, err error) {
	c, err = NewComponents(deps, eventCfg)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = c.Close()
		}
	}()
	err = c.Start(context.Background())
	if err != nil {
		return nil, err
	}
	return c, nil
}

// Start 启动事件后台任务（发件箱抓取/发送/超时回收/DLQ，收件箱死信重投，过期数据清理）。
// 任一后台任务启动失败时取消已启动任务并返回错误。
func (c *Components) Start(parentCtx context.Context) error {
	if c.started {
		return nil
	}
	if c.lockFactory == nil {
		return errors.New("event: 分布式锁工厂未初始化")
	}
	if parentCtx == nil {
		parentCtx = context.Background()
	}

	runCtx, cancel := context.WithCancel(parentCtx)
	c.cancel = cancel

	if c.cfg.PublisherEnabled {
		if c.processor == nil {
			processor, err := outbox.NewProcessor(
				c.cfg, c.outboxRepo, c.dlqHandler, c.kafkaPublisher, c.txExecutor, runCtx,
			)
			if err != nil {
				cancel()
				return err
			}
			c.processor = processor
		}

		fetcher := outbox.NewFetcher(c.cfg, c.outboxRepo, c.processor)
		if err := fetcher.Start(runCtx, c.lockFactory); err != nil {
			cancel()
			return err
		}

		fetchTimeout := outbox.NewFetchTimeout(c.cfg, c.outboxRepo)
		if err := fetchTimeout.Start(runCtx, c.lockFactory); err != nil {
			cancel()
			return err
		}

		if err := c.dlqHandler.Start(runCtx, c.lockFactory); err != nil {
			cancel()
			return err
		}
	} else {
		logger.InfoWithoutCtx("[消息队列事件]发布侧未启用，跳过发件箱后台任务")
	}

	if c.cfg.ConsumerEnabled {
		if c.inboxDlqHandler == nil {
			cancel()
			return errors.New("event: 收件箱死信处理器未初始化")
		}
		if err := c.inboxDlqHandler.Start(runCtx, c.lockFactory); err != nil {
			cancel()
			return err
		}
	}

	if c.retentionCleaner != nil {
		if err := c.retentionCleaner.Start(runCtx, c.lockFactory); err != nil {
			cancel()
			return err
		}
	}

	c.started = true
	logger.InfoWithoutCtx("[消息队列事件]后台任务已启动",
		"serviceName", c.cfg.ServiceName,
		"publisherEnabled", c.cfg.PublisherEnabled,
		"consumerEnabled", c.cfg.ConsumerEnabled,
		"outboxCleanupEnabled", c.cfg.OutboxCleanup.Enabled,
		"inboxCleanupEnabled", c.cfg.InboxCleanup.Enabled,
	)
	return nil
}

// ResetDlqRetryCountInTimeRange 将指定创建时间范围内的 DLQ 重试计数清零，供人工恢复
func (c *Components) ResetDlqRetryCountInTimeRange(ctx context.Context, from, to time.Time) (int64, error) {
	if c.dlqHandler == nil {
		return 0, errors.New("event: 发布侧未启用，无 DLQ 能力")
	}
	return c.dlqHandler.ResetRetryCountInTimeRange(ctx, from, to)
}

// Close 停止事件后台任务并释放资源
func (c *Components) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	if c.processor != nil {
		c.processor.Close()
	}
	return nil
}
