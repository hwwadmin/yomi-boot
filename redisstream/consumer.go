// Package redisstream 提供 Redis Stream 消费组能力
//
// @author zwei
package redisstream

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"gitee.com/hq_project/yomi-boot/appctx"
	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/logger"

	redisv9 "github.com/redis/go-redis/v9"
)

// 消费重试退避常量
const (
	consumerRetryInitialDelay = 500 * time.Millisecond
	consumerRetryMaxDelay     = 10 * time.Second
	consumerRetryJitterMin    = 8
	consumerRetryJitterMax    = 12
	consumerRetryJitterDenom  = 10
	deadOriginalIDField       = "_original_id"
	deadStreamField           = "_dead_from"
)

// ConsumerService 表示 Redis Stream 消费者管理能力
type ConsumerService interface {
	// Start 启动消费循环；同一 id 不可重复注册
	Start(parentCtx context.Context, id string, cfg GroupConfig, handler Handler) error
	// Stop 停止并移除指定消费者
	Stop(id string) error
	// StopAll 停止全部消费者
	StopAll() error
	// Close 停止全部消费者
	Close() error
}

// ConsumerManager 管理多个消费组订阅
type ConsumerManager struct {
	client  *redisv9.Client
	runtime RuntimeConfig
	mu      sync.Mutex
	entries map[string]*consumerEntry
}

var _ ConsumerService = (*ConsumerManager)(nil)

type consumerEntry struct {
	cancel context.CancelFunc
	done   chan struct{}
	ctx    context.Context
}

// NewConsumerManager 创建消费者管理器
func NewConsumerManager(client *redisv9.Client, runtime RuntimeConfig) *ConsumerManager {
	if client == nil {
		panic("redisstream: NewConsumerManager client 不能为 nil")
	}
	return &ConsumerManager{
		client:  client,
		runtime: runtime,
		entries: make(map[string]*consumerEntry),
	}
}

// Start 启动消费者
func (m *ConsumerManager) Start(
	parentCtx context.Context,
	id string,
	cfg GroupConfig,
	handler Handler,
) error {
	if handler == nil {
		return errors.New("redisstream: 消费处理函数不能为空")
	}
	resolved, err := m.resolveGroupConfig(cfg)
	if err != nil {
		return err
	}

	m.mu.Lock()
	if strings.TrimSpace(id) == "" {
		m.mu.Unlock()
		return errors.New("redisstream: 消费者标识不能为空")
	}
	if _, exists := m.entries[id]; exists {
		m.mu.Unlock()
		return errors.Newf("redisstream: 消费者已存在: %s", id)
	}
	ctx, cancel := appctx.WithCancel(parentCtx)
	entry := &consumerEntry{cancel: cancel, done: make(chan struct{}), ctx: ctx}
	m.entries[id] = entry
	m.mu.Unlock()

	go func() {
		defer m.finish(id, entry)
		defer func() {
			if r := recover(); r != nil {
				logger.Error(entry.ctx, "Redis Stream 消费者 goroutine panic", "consumer_id", id, "panic", r)
			}
		}()
		c := &consumer{
			client:  m.client,
			runtime: m.runtime,
			cfg:     resolved,
		}
		_ = c.run(entry.ctx, handler)
	}()

	return nil
}

// Stop 停止指定消费者
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

// StopAll 停止全部消费者
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

// Close 停止全部消费者
func (m *ConsumerManager) Close() error {
	return m.StopAll()
}

func (m *ConsumerManager) finish(id string, entry *consumerEntry) {
	defer close(entry.done)
	m.mu.Lock()
	if current, ok := m.entries[id]; ok && current == entry {
		delete(m.entries, id)
	}
	m.mu.Unlock()
}

func (m *ConsumerManager) resolveGroupConfig(cfg GroupConfig) (resolvedGroupConfig, error) {
	stream := strings.TrimSpace(cfg.Stream)
	group := strings.TrimSpace(cfg.Group)
	consumerName := strings.TrimSpace(cfg.Consumer)
	if stream == "" {
		return resolvedGroupConfig{}, errors.New("redisstream: stream 不能为空")
	}
	if group == "" {
		return resolvedGroupConfig{}, errors.New("redisstream: group 不能为空")
	}
	if consumerName == "" {
		return resolvedGroupConfig{}, errors.New("redisstream: consumer 不能为空")
	}

	start := strings.TrimSpace(cfg.Start)
	if start == "" {
		start = "$"
	}

	count := cfg.Count
	if count <= 0 {
		count = m.runtime.Count
	}
	block := cfg.Block
	if block <= 0 {
		block = m.runtime.Block
	}
	claimMinIdle := cfg.ClaimMinIdle
	if claimMinIdle <= 0 {
		claimMinIdle = m.runtime.ClaimMinIdle
	}
	claimInterval := cfg.ClaimInterval
	if claimInterval <= 0 {
		claimInterval = m.runtime.ClaimInterval
	}

	maxRetries := m.runtime.MaxHandlerRetries
	if cfg.MaxHandlerRetries != nil {
		maxRetries = *cfg.MaxHandlerRetries
	}
	if maxRetries < -1 {
		return resolvedGroupConfig{}, errors.New("redisstream: max_handler_retries 不能小于 -1")
	}

	deadStream := strings.TrimSpace(cfg.DeadStream)
	if deadStream == "" {
		deadStream = stream + ".dead"
	}

	return resolvedGroupConfig{
		logicalStream:     stream,
		streamKey:         m.runtime.KeyPrefix + stream,
		group:             group,
		consumer:          consumerName,
		start:             start,
		count:             count,
		block:             block,
		claimMinIdle:      claimMinIdle,
		claimInterval:     claimInterval,
		deadStreamKey:     m.runtime.KeyPrefix + deadStream,
		maxHandlerRetries: maxRetries,
	}, nil
}

type resolvedGroupConfig struct {
	logicalStream     string
	streamKey         string
	group             string
	consumer          string
	start             string
	count             int64
	block             time.Duration
	claimMinIdle      time.Duration
	claimInterval     time.Duration
	deadStreamKey     string
	maxHandlerRetries int
}

type consumer struct {
	client  *redisv9.Client
	runtime RuntimeConfig
	cfg     resolvedGroupConfig
}

func (c *consumer) run(ctx context.Context, handler Handler) error {
	if err := c.ensureGroup(ctx); err != nil {
		logger.Error(ctx, "创建 Redis Stream 消费组失败",
			"stream", c.cfg.logicalStream,
			"group", c.cfg.group,
			"error", err,
		)
		return err
	}

	claimTicker := time.NewTicker(c.cfg.claimInterval)
	defer claimTicker.Stop()

	// 启动时先尝试回收一次悬挂消息
	c.claimPending(ctx, handler)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-claimTicker.C:
			c.claimPending(ctx, handler)
		default:
		}

		streams, err := c.client.XReadGroup(ctx, &redisv9.XReadGroupArgs{
			Group:    c.cfg.group,
			Consumer: c.cfg.consumer,
			Streams:  []string{c.cfg.streamKey, ">"},
			Count:    c.cfg.count,
			Block:    c.cfg.block,
		}).Result()
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			if errors.Is(err, redisv9.Nil) {
				continue
			}
			logger.Warn(ctx, "Redis Stream XREADGROUP 失败，准备重试", "error", err)
			if waitErr := waitForRetry(ctx, jitterDelay(consumerRetryInitialDelay)); waitErr != nil {
				return waitErr
			}
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				entry := Entry{
					Stream: c.cfg.logicalStream,
					ID:     msg.ID,
					Values: stringifyValues(msg.Values),
				}
				if err := c.processEntry(ctx, handler, entry); err != nil {
					return err
				}
			}
		}
	}
}

func (c *consumer) ensureGroup(ctx context.Context) error {
	err := c.client.XGroupCreateMkStream(ctx, c.cfg.streamKey, c.cfg.group, c.cfg.start).Err()
	if err == nil {
		return nil
	}
	if isBusyGroup(err) {
		return nil
	}
	return errors.Wrap(err, "XGROUP CREATE 失败")
}

func (c *consumer) claimPending(ctx context.Context, handler Handler) {
	start := "0-0"
	for {
		if ctx.Err() != nil {
			return
		}
		messages, next, err := c.client.XAutoClaim(ctx, &redisv9.XAutoClaimArgs{
			Stream:   c.cfg.streamKey,
			Group:    c.cfg.group,
			Consumer: c.cfg.consumer,
			MinIdle:  c.cfg.claimMinIdle,
			Start:    start,
			Count:    c.cfg.count,
		}).Result()
		if err != nil {
			if !errors.Is(err, redisv9.Nil) && !errors.Is(err, context.Canceled) {
				logger.Warn(ctx, "Redis Stream XAUTOCLAIM 失败",
					"stream", c.cfg.logicalStream,
					"group", c.cfg.group,
					"error", err,
				)
			}
			return
		}
		for _, msg := range messages {
			entry := Entry{
				Stream: c.cfg.logicalStream,
				ID:     msg.ID,
				Values: stringifyValues(msg.Values),
			}
			if err := c.processEntry(ctx, handler, entry); err != nil {
				return
			}
		}
		if next == "0-0" || len(messages) == 0 {
			return
		}
		start = next
	}
}

func (c *consumer) processEntry(ctx context.Context, handler Handler, entry Entry) error {
	retries := 0
	delay := jitterDelay(consumerRetryInitialDelay)
	for {
		result := invokeHandler(ctx, handler, entry)
		switch result {
		case HandleAck:
			return c.ackWithRetry(ctx, entry.ID)
		case HandleDead:
			if err := c.moveToDead(ctx, entry); err != nil {
				logger.Error(ctx, "写入 Redis Stream dead 失败，保留在 PEL 并重试",
					"stream", entry.Stream,
					"id", entry.ID,
					"error", err,
				)
				if waitErr := waitForRetry(ctx, delay); waitErr != nil {
					return waitErr
				}
				delay = nextRetryDelay(delay)
				continue
			}
			return c.ackWithRetry(ctx, entry.ID)
		case HandleRetry:
			if c.cfg.maxHandlerRetries >= 0 && retries >= c.cfg.maxHandlerRetries {
				logger.Error(ctx, "Redis Stream 消费重试超限，转入 dead",
					"stream", entry.Stream,
					"id", entry.ID,
					"retries", retries,
					"max_retries", c.cfg.maxHandlerRetries,
				)
				if err := c.moveToDead(ctx, entry); err != nil {
					logger.Error(ctx, "写入 Redis Stream dead 失败，保留在 PEL 并重试",
						"stream", entry.Stream,
						"id", entry.ID,
						"error", err,
					)
					if waitErr := waitForRetry(ctx, delay); waitErr != nil {
						return waitErr
					}
					delay = nextRetryDelay(delay)
					continue
				}
				return c.ackWithRetry(ctx, entry.ID)
			}
			logger.Warn(ctx, "Redis Stream 消费将重试",
				"stream", entry.Stream,
				"id", entry.ID,
				"retries", retries,
			)
			retries++
			if err := waitForRetry(ctx, delay); err != nil {
				return err
			}
			delay = nextRetryDelay(delay)
		default:
			logger.Warn(ctx, "Redis Stream 未知 HandleResult，按 Retry 处理",
				"stream", entry.Stream,
				"id", entry.ID,
				"result", result,
			)
			retries++
			if err := waitForRetry(ctx, delay); err != nil {
				return err
			}
			delay = nextRetryDelay(delay)
		}
	}
}

func (c *consumer) ackWithRetry(ctx context.Context, id string) error {
	delay := jitterDelay(consumerRetryInitialDelay)
	for {
		err := c.client.XAck(ctx, c.cfg.streamKey, c.cfg.group, id).Err()
		if err == nil {
			return nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		logger.Warn(ctx, "Redis Stream XACK 失败，准备重试", "id", id, "error", err)
		if waitErr := waitForRetry(ctx, delay); waitErr != nil {
			return waitErr
		}
		delay = nextRetryDelay(delay)
	}
}

func (c *consumer) moveToDead(ctx context.Context, entry Entry) error {
	values := make(map[string]any, len(entry.Values)+2)
	for k, v := range entry.Values {
		values[k] = v
	}
	values[deadOriginalIDField] = entry.ID
	values[deadStreamField] = entry.Stream

	args := &redisv9.XAddArgs{
		Stream: c.cfg.deadStreamKey,
		Values: values,
	}
	if c.runtime.DefaultMaxLen > 0 {
		args.MaxLen = c.runtime.DefaultMaxLen
		args.Approx = true
	}
	if err := c.client.XAdd(ctx, args).Err(); err != nil {
		return errors.Wrap(err, "Redis Stream dead XADD 失败")
	}
	return nil
}

func invokeHandler(ctx context.Context, handler Handler, entry Entry) (result HandleResult) {
	defer func() {
		if r := recover(); r != nil {
			result = HandleRetry
			logger.Error(ctx, "Redis Stream 消费 handler panic",
				"panic", r,
				"stream", entry.Stream,
				"id", entry.ID,
			)
		}
	}()
	return handler(ctx, entry)
}

func stringifyValues(values map[string]any) map[string]string {
	out := make(map[string]string, len(values))
	for k, v := range values {
		out[k] = valueToString(v)
	}
	return out
}

func valueToString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return fmt.Sprint(t)
	}
}

func isBusyGroup(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "BUSYGROUP")
}

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

func jitterDelay(base time.Duration) time.Duration {
	n := consumerRetryJitterMin + rand.IntN(consumerRetryJitterMax-consumerRetryJitterMin+1)
	return base * time.Duration(n) / time.Duration(consumerRetryJitterDenom)
}

func nextRetryDelay(current time.Duration) time.Duration {
	next := current * 2
	if next > consumerRetryMaxDelay {
		next = consumerRetryMaxDelay
	}
	return jitterDelay(next)
}
