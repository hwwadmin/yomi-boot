// Package kafka 提供 Kafka 消息消费封装与管理能力
//
// @author zwei
package kafka

import (
	"context"
	"math/rand/v2"
	"sync"
	"time"
	"yomi-boot/appctx"
	"yomi-boot/errors"
	"yomi-boot/logger"

	kafkago "github.com/segmentio/kafka-go"
)

// 消费者重试退避常量
const (
	consumerRetryInitialDelay = 500 * time.Millisecond // consumerRetryInitialDelay 是消费重试初始等待时长
	consumerRetryMaxDelay     = 10 * time.Second       // consumerRetryMaxDelay 是消费重试最大等待时长
	consumerRetryJitterMin    = 8                      // consumerRetryJitterMin 是退避抖动下限比例分子（分母 10，即 80%）
	consumerRetryJitterMax    = 12                     // consumerRetryJitterMax 是退避抖动上限比例分子（分母 10，即 120%）
	consumerRetryJitterDenom  = 10                     // consumerRetryJitterDenom 是退避抖动比例分母
)

// Consumer 封装单个 Kafka 消费者实例
type Consumer struct {
	reader            *kafkago.Reader // reader 是底层 Kafka 消费者
	maxHandlerRetries int             // maxHandlerRetries 是 handler 返回 Retry 时的最大重试次数，-1 表示无限
	closeMu           sync.Mutex      // closeMu 保护 closed 并发调用
	closed            bool            // closed 标记是否已关闭
}

// ConsumerManager 管理多个消费者实例的启动、停止与释放
type ConsumerManager struct {
	runtimeCfg KafkaRuntimeConfig        // runtimeCfg 是 Kafka 运行时配置
	mu         sync.Mutex                // mu 保护 entries 并发访问
	entries    map[string]*consumerEntry // entries 是按标识管理的消费者实例
}

// ConsumerService 表示 Kafka 消费者管理能力接口
type ConsumerService interface {
	// Start 启动消费者，handler 通过 ConsumeResult 显式返回处理结果与 ack 决策
	Start(parentCtx context.Context, id string, cfg ConsumerConfig, handler ConsumeHandler) error
	// Stop 停止并移除指定消费者
	Stop(id string) error
	// StopAll 停止并移除全部消费者
	StopAll() error
	// Close 停止全部消费者并释放管理器
	Close() error
}

var _ ConsumerService = (*ConsumerManager)(nil)

// consumerEntry 表示一个受管消费者的运行态
type consumerEntry struct {
	consumer *Consumer          // consumer 是消费者实例
	cancel   context.CancelFunc // cancel 用于停止消费循环
	done     chan struct{}      // done 在消费者退出后关闭
	ctx      context.Context    // ctx 是消费者独立运行上下文
}

// NewConsumerManager 创建消费者管理器
func NewConsumerManager(runtimeCfg KafkaRuntimeConfig) ConsumerService {
	return &ConsumerManager{
		runtimeCfg: runtimeCfg,
		entries:    make(map[string]*consumerEntry),
	}
}

// Start 启动消费者，handler 通过 ConsumeResult 显式返回处理结果与 ack 决策
func (m *ConsumerManager) Start(
	parentCtx context.Context,
	id string,
	cfg ConsumerConfig,
	handler ConsumeHandler,
) error {
	if handler == nil {
		return errors.New("消费处理函数不能为空")
	}

	consumer, entry, err := m.register(parentCtx, id, cfg)
	if err != nil {
		return err
	}

	go func() {
		defer m.finish(id, entry)
		defer func() {
			if r := recover(); r != nil {
				logger.Error(entry.ctx, "Kafka 消费者 goroutine panic", "consumer_id", id, "panic", r)
			}
		}()
		_ = consumer.Run(entry.ctx, handler)
	}()

	return nil
}

// Stop 停止并移除指定消费者；若消费者已停止或不存在则幂等返回 nil
func (m *ConsumerManager) Stop(id string) error {
	m.mu.Lock()
	entry, ok := m.entries[id]
	if !ok {
		m.mu.Unlock()
		return nil
	}
	delete(m.entries, id)
	m.mu.Unlock()

	entry.cancel()
	<-entry.done
	return nil
}

// StopAll 停止并移除全部消费者
func (m *ConsumerManager) StopAll() error {
	m.mu.Lock()
	ids := make([]string, 0, len(m.entries))
	for id := range m.entries {
		ids = append(ids, id)
	}
	m.mu.Unlock()

	var joined error
	for _, id := range ids {
		if err := m.Stop(id); err != nil {
			joined = errors.Join(joined, err)
		}
	}
	return joined
}

// Close 停止全部消费者并释放管理器
func (m *ConsumerManager) Close() error {
	return m.StopAll()
}

// register 创建并登记消费者实例，同一标识不可重复注册
func (m *ConsumerManager) register(
	parentCtx context.Context,
	id string,
	cfg ConsumerConfig,
) (*Consumer, *consumerEntry, error) {
	if id == "" {
		return nil, nil, errors.New("kafka 消费者标识不能为空")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.entries[id]; exists {
		return nil, nil, errors.Newf("kafka 消费者已存在: %s", id)
	}

	consumer, err := newConsumer(m.runtimeCfg, cfg)
	if err != nil {
		return nil, nil, err
	}

	ctx, cancel := appctx.WithCancel(parentCtx)
	entry := &consumerEntry{
		consumer: consumer,
		cancel:   cancel,
		done:     make(chan struct{}),
		ctx:      ctx,
	}
	m.entries[id] = entry

	return consumer, entry, nil
}

// finish 在消费者协程退出时清理登记信息并关闭资源
func (m *ConsumerManager) finish(id string, entry *consumerEntry) {
	defer close(entry.done)
	defer func() {
		_ = entry.consumer.Close()
	}()

	m.mu.Lock()
	if current, ok := m.entries[id]; ok && current == entry {
		delete(m.entries, id)
	}
	m.mu.Unlock()
}

// newConsumer 根据运行时配置与消费参数创建消费者实例
func newConsumer(runtimeCfg KafkaRuntimeConfig, cfg ConsumerConfig) (*Consumer, error) {
	readerCfg, err := buildReaderConfig(runtimeCfg, cfg)
	if err != nil {
		return nil, err
	}

	maxHandlerRetries, err := effectiveMaxHandlerRetries(cfg)
	if err != nil {
		return nil, err
	}

	return &Consumer{
		reader:            kafkago.NewReader(readerCfg),
		maxHandlerRetries: maxHandlerRetries,
	}, nil
}

// Run 阻塞消费消息，Ack 为 false 时在同一条消息上退避重试，Ack 为 true 时提交偏移量
func (c *Consumer) Run(ctx context.Context, handler ConsumeHandler) error {
	if handler == nil {
		return errors.New("消费处理函数不能为空")
	}

	for {
		kafkaMsg, err := c.fetchMessageWithRetry(ctx)
		if err != nil {
			return err
		}

		msg := fromKafkaMessage(kafkaMsg)
		if err := c.processMessageWithRetry(ctx, handler, msg, kafkaMsg); err != nil {
			return err
		}
	}
}

// processMessageWithRetry 处理单条消息，Ack 为 false 时在同一条消息上退避重试
func (c *Consumer) processMessageWithRetry(
	ctx context.Context,
	handler ConsumeHandler,
	msg Message,
	kafkaMsg kafkago.Message,
) error {
	retries := 0
	delay := jitterDelay(consumerRetryInitialDelay)
	for {
		result := normalizeConsumeResult(ctx, invokeConsumeHandler(ctx, handler, msg))
		if result.Ack {
			if result.Status == ConsumeStatusDrop {
				logConsumeDrop(ctx, msg, result)
			}
			return c.commitMessageWithRetry(ctx, kafkaMsg)
		}

		if c.maxHandlerRetries >= 0 && retries >= c.maxHandlerRetries {
			logConsumeDropExceeded(ctx, msg, retries, c.maxHandlerRetries)
			return c.commitMessageWithRetry(ctx, kafkaMsg)
		}

		logConsumeRetry(ctx, msg, result)
		retries++
		if err := waitForRetry(ctx, delay); err != nil {
			return err
		}
		delay = nextRetryDelay(delay)
	}
}

// Close 释放消费者连接，幂等安全
func (c *Consumer) Close() error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	if err := c.reader.Close(); err != nil {
		return errors.Wrap(err, "关闭 Kafka 消费者失败")
	}
	return nil
}

// invokeConsumeHandler 调用消费 handler 并捕获 panic，按可重试失败处理
func invokeConsumeHandler(ctx context.Context, handler ConsumeHandler, msg Message) (result ConsumeResult) {
	defer func() {
		if r := recover(); r != nil {
			result = Retry()
			logger.Error(ctx, "Kafka 消费 handler panic",
				"panic", r,
				"topic", msg.Topic,
				"partition", msg.Partition,
				"offset", msg.Offset,
			)
		}
	}()
	return handler(ctx, msg)
}

// normalizeConsumeResult 校验并规范化 handler 返回结果，非法组合按保守策略处理
func normalizeConsumeResult(ctx context.Context, result ConsumeResult) ConsumeResult {
	switch result.Status {
	case ConsumeStatusRetry:
		if result.Ack {
			logger.Warn(ctx,
				"Kafka 消费结果非法：ConsumeStatusRetry 不应与 Ack=true 组合，已按不提交处理",
				"status", result.Status,
			)
			result.Ack = false
		}
	case ConsumeStatusOK, ConsumeStatusDrop:
		if !result.Ack {
			logger.Warn(ctx,
				"Kafka 消费结果非法：成功或丢弃状态不应与 Ack=false 组合，已按可重试处理",
				"status", result.Status,
			)
			result.Status = ConsumeStatusRetry
			result.Ack = false
		}
	default:
		if result.Ack {
			logger.Warn(ctx,
				"Kafka 消费结果包含未知状态，已按成功提交处理",
				"status", result.Status,
			)
			result.Status = ConsumeStatusOK
		} else {
			logger.Warn(ctx,
				"Kafka 消费结果包含未知状态，已按可重试处理",
				"status", result.Status,
			)
			result.Status = ConsumeStatusRetry
			result.Ack = false
		}
	}
	return result
}

// fetchMessageWithRetry 拉取消息，瞬时故障时按指数退避重试直至成功或上下文结束
func (c *Consumer) fetchMessageWithRetry(ctx context.Context) (kafkago.Message, error) {
	var msg kafkago.Message
	err := retryWithBackoff(ctx, "拉取 Kafka 消息失败，准备重试", func() error {
		fetched, err := c.reader.FetchMessage(ctx)
		if err != nil {
			return err
		}
		msg = fetched
		return nil
	})
	return msg, err
}

// commitMessageWithRetry 提交偏移量，瞬时故障时按指数退避重试直至成功或上下文结束
func (c *Consumer) commitMessageWithRetry(ctx context.Context, msg kafkago.Message) error {
	return retryWithBackoff(ctx, "提交 Kafka 偏移量失败，准备重试", func() error {
		return c.reader.CommitMessages(ctx, msg)
	})
}

// retryWithBackoff 在瞬时故障时按指数退避重试，上下文取消或超时时立即返回
func retryWithBackoff(ctx context.Context, warnMsg string, operation func() error) error {
	delay := jitterDelay(consumerRetryInitialDelay)
	for {
		err := operation()
		if err == nil {
			return nil
		}
		if isConsumerContextDone(err) {
			return err
		}

		logger.Warn(ctx, warnMsg, "error", err, "retry_after", delay)
		if err := waitForRetry(ctx, delay); err != nil {
			return err
		}
		delay = nextRetryDelay(delay)
	}
}

// isConsumerContextDone 判断错误是否由消费上下文取消或超时引起
func isConsumerContextDone(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// waitForRetry 等待重试间隔，上下文结束时返回错误
func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// nextRetryDelay 计算下一次重试等待时长，采用指数退避、设上限并施加抖动
func nextRetryDelay(current time.Duration) time.Duration {
	next := current * 2
	if next > consumerRetryMaxDelay {
		next = consumerRetryMaxDelay
	}
	return jitterDelay(next)
}

// jitterDelay 将等待时长抖动到 [delay*80%, delay*120%] 区间，避免多消费者同时重试惊群
func jitterDelay(delay time.Duration) time.Duration {
	if delay <= 0 {
		return delay
	}

	min := delay * consumerRetryJitterMin / consumerRetryJitterDenom
	max := delay * consumerRetryJitterMax / consumerRetryJitterDenom
	if max <= min {
		return delay
	}
	return min + time.Duration(rand.Int64N(int64(max-min)+1))
}

// logConsumeRetry 记录消费处理需重试的日志，偏移量未提交时将在同一会话内重试同一条消息
func logConsumeRetry(ctx context.Context, msg Message, result ConsumeResult) {
	logger.Warn(ctx,
		"Kafka 消费处理需重试，偏移量未提交",
		"status", result.Status,
		"topic", msg.Topic,
		"partition", msg.Partition,
		"offset", msg.Offset,
	)
}

// logConsumeDrop 记录消息丢弃日志，偏移量提交后将跳过当前消息
func logConsumeDrop(ctx context.Context, msg Message, result ConsumeResult) {
	logger.Warn(ctx,
		"Kafka 消息已丢弃，偏移量已提交",
		"status", result.Status,
		"topic", msg.Topic,
		"partition", msg.Partition,
		"offset", msg.Offset,
	)
}

// logConsumeDropExceeded 记录 handler 重试超限后自动丢弃的告警日志
func logConsumeDropExceeded(ctx context.Context, msg Message, retries int, maxRetries int) {
	logger.Error(ctx,
		"Kafka 消息重试次数已超限，已自动丢弃",
		"topic", msg.Topic,
		"partition", msg.Partition,
		"offset", msg.Offset,
		"retries", retries,
		"max_handler_retries", maxRetries,
	)
}

// fromKafkaMessage 将 kafka-go 消息转换为业务消息结构
func fromKafkaMessage(msg kafkago.Message) Message {
	headers := make([]Header, 0, len(msg.Headers))
	for _, header := range msg.Headers {
		headers = append(headers, Header{
			Key:   header.Key,
			Value: header.Value,
		})
	}

	return Message{
		Topic:     msg.Topic,
		Partition: msg.Partition,
		Offset:    msg.Offset,
		Key:       msg.Key,
		Value:     msg.Value,
		Time:      msg.Time,
		Headers:   headers,
	}
}
