package kafka

import (
	"context"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/appctx"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/logger"

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

// consumerRuntime 表示消费者管理器的连接与默认拉取参数
type consumerRuntime struct {
	brokers     []string        // brokers 是 Kafka broker 地址列表
	minBytes    int             // minBytes 是单次拉取最小字节数默认值
	maxBytes    int             // maxBytes 是单次拉取最大字节数默认值
	maxWait     time.Duration   // maxWait 是拉取等待时长默认值
	startOffset int64           // startOffset 是消费起始偏移量默认值
	dialer      *kafkago.Dialer // dialer 是带 SASL/TLS 能力的连接拨号器
}

// consumer 封装单个 Kafka 消费者实例
type consumer struct {
	reader            *kafkago.Reader // reader 是底层 Kafka 消费者
	maxHandlerRetries int             // maxHandlerRetries 是 handler 返回 Retry 时的最大重试次数，-1 表示无限
	closeMu           sync.Mutex      // closeMu 保护 closed 并发调用
	closed            bool            // closed 标记是否已关闭
}

// consumerManager 管理多个消费者实例的启动、停止与释放
type consumerManager struct {
	runtimeCfg consumerRuntime           // runtimeCfg 是 Kafka 消费者运行时配置
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

var _ ConsumerService = (*consumerManager)(nil)

// consumerEntry 表示一个受管消费者的运行态
type consumerEntry struct {
	consumer *consumer          // consumer 是消费者实例
	cancel   context.CancelFunc // cancel 用于停止消费循环
	done     chan struct{}      // done 在消费者退出后关闭
	ctx      context.Context    // ctx 是消费者独立运行上下文
}

// NewConsumerManager 根据消费者管理器配置创建消费者管理器；消费者在 Start 时才建立连接
func NewConsumerManager(cfg ConsumerManagerConfig) (ConsumerService, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	conn, err := newConnection(cfg.Connection)
	if err != nil {
		return nil, err
	}
	if err := validateConnection(conn, cfg.Connection.DialTimeout()); err != nil {
		return nil, errors.Wrap(err, "kafka连接验证失败")
	}

	logger.InfoWithoutCtx(
		"初始化 Kafka 消费者管理器成功",
		"brokers", strings.Join(conn.brokers, ","),
		"security_protocol", NormalizeSecurityProtocol(cfg.Connection.SecurityProtocol),
		"sasl_mechanism", NormalizeSASLMechanism(cfg.Connection.SASLMechanism),
		"min_bytes", cfg.MinBytes,
		"max_bytes", cfg.MaxBytes,
		"max_wait_seconds", cfg.MaxWaitSeconds,
		"start_offset", cfg.StartOffset,
	)

	return &consumerManager{
		runtimeCfg: newConsumerRuntime(cfg, conn),
		entries:    make(map[string]*consumerEntry),
	}, nil
}

// newConsumerRuntime 将消费者管理器配置转换为运行时默认参数
func newConsumerRuntime(cfg ConsumerManagerConfig, conn connection) consumerRuntime {
	startOffset := kafkago.FirstOffset
	if strings.TrimSpace(cfg.StartOffset) == "latest" {
		startOffset = kafkago.LastOffset
	}
	return consumerRuntime{
		brokers:     conn.brokers,
		minBytes:    cfg.MinBytes,
		maxBytes:    cfg.MaxBytes,
		maxWait:     cfg.MaxWait(),
		startOffset: startOffset,
		dialer:      conn.dialer,
	}
}

// Start 启动消费者，handler 通过 ConsumeResult 显式返回处理结果与 ack 决策
func (m *consumerManager) Start(
	parentCtx context.Context,
	id string,
	cfg ConsumerConfig,
	handler ConsumeHandler,
) error {
	if handler == nil {
		return errors.New("消费处理函数不能为空")
	}

	instance, entry, err := m.register(parentCtx, id, cfg)
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
		_ = instance.run(entry.ctx, handler)
	}()

	return nil
}

// Stop 停止并移除指定消费者；若消费者已停止或不存在则幂等返回 nil
func (m *consumerManager) Stop(id string) error {
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
func (m *consumerManager) StopAll() error {
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
func (m *consumerManager) Close() error {
	return m.StopAll()
}

// register 创建并登记消费者实例，同一标识不可重复注册
func (m *consumerManager) register(
	parentCtx context.Context,
	id string,
	cfg ConsumerConfig,
) (*consumer, *consumerEntry, error) {
	if id == "" {
		return nil, nil, errors.New("kafka 消费者标识不能为空")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.entries[id]; exists {
		return nil, nil, errors.Newf("kafka 消费者已存在: %s", id)
	}

	instance, err := newConsumer(m.runtimeCfg, cfg)
	if err != nil {
		return nil, nil, err
	}

	ctx, cancel := appctx.WithCancel(parentCtx)
	entry := &consumerEntry{
		consumer: instance,
		cancel:   cancel,
		done:     make(chan struct{}),
		ctx:      ctx,
	}
	m.entries[id] = entry

	return instance, entry, nil
}

// finish 在消费者协程退出时清理登记信息并关闭资源
func (m *consumerManager) finish(id string, entry *consumerEntry) {
	defer close(entry.done)
	defer func() {
		_ = entry.consumer.close()
	}()

	m.mu.Lock()
	if current, ok := m.entries[id]; ok && current == entry {
		delete(m.entries, id)
	}
	m.mu.Unlock()
}

// newConsumer 根据运行时配置与消费参数创建消费者实例
func newConsumer(runtimeCfg consumerRuntime, cfg ConsumerConfig) (*consumer, error) {
	readerCfg, err := buildReaderConfig(runtimeCfg, cfg)
	if err != nil {
		return nil, err
	}

	maxHandlerRetries, err := effectiveMaxHandlerRetries(cfg)
	if err != nil {
		return nil, err
	}

	return &consumer{
		reader:            kafkago.NewReader(readerCfg),
		maxHandlerRetries: maxHandlerRetries,
	}, nil
}

// run 阻塞消费消息，Ack 为 false 时在同一条消息上退避重试，Ack 为 true 时提交偏移量
func (c *consumer) run(ctx context.Context, handler ConsumeHandler) error {
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
func (c *consumer) processMessageWithRetry(
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

// close 释放消费者连接，幂等安全
func (c *consumer) close() error {
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
func (c *consumer) fetchMessageWithRetry(ctx context.Context) (kafkago.Message, error) {
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
func (c *consumer) commitMessageWithRetry(ctx context.Context, msg kafkago.Message) error {
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

// buildReaderConfig 根据运行时配置与消费参数构建 Reader 配置
func buildReaderConfig(runtimeCfg consumerRuntime, cfg ConsumerConfig) (kafkago.ReaderConfig, error) {
	if err := validateConsumerConfig(cfg); err != nil {
		return kafkago.ReaderConfig{}, err
	}

	readerCfg := kafkago.ReaderConfig{
		Brokers:     runtimeCfg.brokers,
		GroupID:     cfg.GroupID,
		GroupTopics: cfg.Topics,
		MinBytes:    firstPositive(cfg.MinBytes, runtimeCfg.minBytes, 1),
		MaxBytes:    firstPositive(cfg.MaxBytes, runtimeCfg.maxBytes, 10e6),
		MaxWait:     firstDuration(cfg.MaxWait, runtimeCfg.maxWait, time.Second),
		StartOffset: firstOffset(cfg.StartOffset, runtimeCfg.startOffset),
		Dialer:      runtimeCfg.dialer,
	}

	return readerCfg, nil
}

// validateConsumerConfig 校验消费者订阅参数
func validateConsumerConfig(cfg ConsumerConfig) error {
	if len(cfg.Topics) == 0 {
		return errors.New("kafka 消费主题不能为空")
	}
	if strings.TrimSpace(cfg.GroupID) == "" {
		return errors.New("kafka 消费组不能为空")
	}
	if _, err := effectiveMaxHandlerRetries(cfg); err != nil {
		return err
	}
	return nil
}

// effectiveMaxHandlerRetries 返回生效的 handler 最大重试次数，未设置时默认 -1（无限重试）
func effectiveMaxHandlerRetries(cfg ConsumerConfig) (int, error) {
	retries := -1
	if cfg.MaxHandlerRetries != nil {
		retries = *cfg.MaxHandlerRetries
	}
	if retries < -1 {
		return 0, errors.New("kafka 消费 handler 最大重试次数不能小于 -1")
	}
	return retries, nil
}

// firstPositive 返回第一个大于 0 的整数值，均无效时返回 0
func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

// firstDuration 返回第一个大于 0 的时长值，均无效时返回 0
func firstDuration(values ...time.Duration) time.Duration {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

// firstOffset 返回第一个非 0 的偏移量，均无效时返回 kafka.FirstOffset
func firstOffset(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return kafkago.FirstOffset
}
