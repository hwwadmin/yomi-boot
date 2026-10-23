// Package redisstream 提供 Redis Stream 消费组能力
//
// @author zwei
package redisstream

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/appctx"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/logger"

	redisv9 "github.com/redis/go-redis/v9"
)

// 消费重试退避常量
const (
	consumerRetryInitialDelay = 500 * time.Millisecond
	consumerRetryMaxDelay     = 10 * time.Second
	consumerRetryJitterMin    = 8
	consumerRetryJitterMax    = 12
	consumerRetryJitterDenom  = 10
	ackRetryMaxAttempts       = 5 // ackRetryMaxAttempts 是 XACK 最大尝试次数（含首次），超限留 PEL
	ackRetryInitialDelay      = 100 * time.Millisecond
	ackRetryMaxDelay          = 1 * time.Second // ack 退避上限，避免读循环被长时间拖住
)

// deadMeta 是写入死信独立 meta 字段的 JSON 结构
type deadMeta struct {
	OriginalID string `json:"original_id"` // OriginalID 是原 stream 中的 entry id
	Stream     string `json:"stream"`      // Stream 是逻辑 stream 名
}

// ConsumerService 表示 Redis Stream 消费者管理能力
type ConsumerService interface {
	// Start 同步确保消费组可用后启动消费循环；建组失败或管理器已 Close 时返回 error，同一 id 不可重复注册
	Start(parentCtx context.Context, id string, cfg GroupConfig, handler Handler) error
	// Stop 停止并移除指定消费者；等待退出受 stop_timeout 限制，超时后登记 orphan 并返回（同 id 不可再 Start 直至协程退出）
	Stop(id string) error
	// StopAll 停止全部消费者
	StopAll() error
	// Close 停止全部消费者并标记管理器已关闭；之后 Start 将返回 error
	Close() error
}

// ConsumerManager 管理多个消费组订阅
type ConsumerManager struct {
	client      *redisv9.Client           // client 是底层 Redis 客户端
	runtime     RuntimeConfig             // runtime 是运行时默认参数
	stopTimeout time.Duration             // stopTimeout 是 Stop 等待协程退出的超时
	mu          sync.Mutex                // mu 保护 entries、orphans 与 closed
	entries     map[string]*consumerEntry // entries 是按标识管理的活跃消费者
	orphans     map[string]*consumerEntry // orphans 是 Stop 超时后仍存活、禁止同 id 再 Start 的协程
	closed      bool                      // closed 标记 Close 之后禁止再 Start
}

var _ ConsumerService = (*ConsumerManager)(nil)

// consumerEntry 表示一个受管消费者的运行态
type consumerEntry struct {
	cancel context.CancelFunc // cancel 用于停止消费循环
	done   chan struct{}      // done 在消费者退出后关闭
	ctx    context.Context    // ctx 是消费者独立运行上下文
}

// NewConsumerManager 创建消费者管理器
func NewConsumerManager(client *redisv9.Client, runtime RuntimeConfig) *ConsumerManager {
	if client == nil {
		panic("redisstream: NewConsumerManager client 不能为 nil")
	}
	stopTimeout := runtime.StopTimeout
	if stopTimeout <= 0 {
		stopTimeout = time.Duration(defaultStopTimeoutMS) * time.Millisecond
	}
	return &ConsumerManager{
		client:      client,
		runtime:     runtime,
		stopTimeout: stopTimeout,
		entries:     make(map[string]*consumerEntry),
		orphans:     make(map[string]*consumerEntry),
	}
}

// Start 启动消费者；同步创建消费组，失败时返回 error 且不启动消费循环
func (m *ConsumerManager) Start(
	parentCtx context.Context,
	id string,
	cfg GroupConfig,
	handler Handler,
) error {
	if handler == nil {
		return errors.New("redisstream: 消费处理函数不能为空")
	}
	if strings.TrimSpace(id) == "" {
		return errors.New("redisstream: 消费者标识不能为空")
	}
	resolved, err := m.resolveGroupConfig(cfg)
	if err != nil {
		return err
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errors.New("redisstream: ConsumerManager 已关闭")
	}
	if _, exists := m.entries[id]; exists {
		m.mu.Unlock()
		return errors.Newf("redisstream: 消费者已存在: %s", id)
	}
	if _, exists := m.orphans[id]; exists {
		m.mu.Unlock()
		return errors.Newf("redisstream: 消费者仍在停止中（上次 Stop 超时，协程尚未退出）: %s", id)
	}
	m.mu.Unlock()

	c := &consumer{
		client:  m.client,
		runtime: m.runtime,
		cfg:     resolved,
	}
	if err := c.ensureGroup(parentCtx); err != nil {
		return err
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errors.New("redisstream: ConsumerManager 已关闭")
	}
	if _, exists := m.entries[id]; exists {
		m.mu.Unlock()
		return errors.Newf("redisstream: 消费者已存在: %s", id)
	}
	if _, exists := m.orphans[id]; exists {
		m.mu.Unlock()
		return errors.Newf("redisstream: 消费者仍在停止中（上次 Stop 超时，协程尚未退出）: %s", id)
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
		if err := c.run(entry.ctx, handler); err != nil {
			logger.Warn(entry.ctx, "Redis Stream 消费者退出", "consumer_id", id, "error", err)
		}
	}()

	return nil
}

// Stop 停止指定消费者；等待退出超时后登记为 orphan 并返回，避免 handler 不响应取消时永久挂起。
// 超时后同 id 不可再 Start，直至旧协程退出（finish 清理 orphan）。
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
	timer := time.NewTimer(m.stopTimeout)
	defer timer.Stop()
	select {
	case <-entry.done:
		return nil
	case <-timer.C:
		m.mu.Lock()
		select {
		case <-entry.done:
			// 超时临界点协程已退出，无需占用 orphan
		default:
			m.orphans[id] = entry
		}
		m.mu.Unlock()
		logger.Error(entry.ctx, "Redis Stream 消费者停止超时，协程仍存活；同 id 禁止再 Start 直至退出",
			"consumer_id", id,
			"timeout", m.stopTimeout,
		)
		return nil
	}
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

// Close 停止全部消费者并标记已关闭；幂等，之后 Start 将失败
func (m *ConsumerManager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	m.mu.Unlock()
	return m.StopAll()
}

// finish 在消费者协程退出时清理 entries/orphans 登记并关闭 done
func (m *ConsumerManager) finish(id string, entry *consumerEntry) {
	defer close(entry.done)
	m.mu.Lock()
	if current, ok := m.entries[id]; ok && current == entry {
		delete(m.entries, id)
	}
	if current, ok := m.orphans[id]; ok && current == entry {
		delete(m.orphans, id)
	}
	m.mu.Unlock()
}

// resolveGroupConfig 校验并解析消费组参数，补齐全局默认值
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

	deadStream := strings.TrimSpace(cfg.DeadStream)
	if deadStream == "" {
		deadStream = stream + ".dead"
	}

	return resolvedGroupConfig{
		logicalStream: stream,
		streamKey:     m.runtime.KeyPrefix + stream,
		group:         group,
		consumer:      consumerName,
		start:         start,
		count:         count,
		block:         block,
		claimMinIdle:  claimMinIdle,
		claimInterval: claimInterval,
		deadStreamKey: m.runtime.KeyPrefix + deadStream,
	}, nil
}

// resolvedGroupConfig 是已解析的消费组运行参数
type resolvedGroupConfig struct {
	logicalStream string        // logicalStream 是逻辑 stream 名
	streamKey     string        // streamKey 是带前缀的 Redis key
	group         string        // group 是消费组名
	consumer      string        // consumer 是本实例消费者名
	start         string        // start 是建组起始 id
	count         int64         // count 是单次拉取条数
	block         time.Duration // block 是 XREADGROUP 阻塞时长
	claimMinIdle  time.Duration // claimMinIdle 是 PEL 认领最小空闲
	claimInterval time.Duration // claimInterval 是 PEL 回收周期
	deadStreamKey string        // deadStreamKey 是死信 stream 的 Redis key
}

// consumer 表示单个消费组订阅的运行实例
type consumer struct {
	client  *redisv9.Client     // client 是底层 Redis 客户端
	runtime RuntimeConfig       // runtime 是运行时默认参数
	cfg     resolvedGroupConfig // cfg 是已解析的订阅参数
}

// run 阻塞执行消费循环，并启动独立 claim 协程回收 PEL。
// processMu 仅串行化 Handler 调用；XACK/dead 等 Redis 收尾在锁外执行，避免故障退避拖死 claim。
func (c *consumer) run(ctx context.Context, handler Handler) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var processMu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				logger.Error(ctx, "Redis Stream claimLoop goroutine panic",
					"stream", c.cfg.logicalStream,
					"group", c.cfg.group,
					"panic", r,
				)
			}
		}()
		c.claimLoop(ctx, handler, &processMu)
	}()
	defer wg.Wait()

	delay := jitterDelay(consumerRetryInitialDelay)
	for {
		if err := ctx.Err(); err != nil {
			return err
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
				// 阻塞超时无消息说明 Redis 仍可用，重置退避
				delay = jitterDelay(consumerRetryInitialDelay)
				continue
			}
			logger.Warn(ctx, "Redis Stream XREADGROUP 失败，准备重试",
				"error", err,
				"retry_after", delay,
			)
			if waitErr := waitForRetry(ctx, delay); waitErr != nil {
				return waitErr
			}
			delay = nextRetryDelay(delay)
			continue
		}

		delay = jitterDelay(consumerRetryInitialDelay)
		for _, stream := range streams {
			if err := c.processMessages(ctx, handler, stream.Messages, &processMu); err != nil {
				return err
			}
		}
	}
}

// claimLoop 独立周期执行 XAUTOCLAIM；与读循环共享 processMu 保证 handler 串行
func (c *consumer) claimLoop(ctx context.Context, handler Handler, processMu *sync.Mutex) {
	ticker := time.NewTicker(c.cfg.claimInterval)
	defer ticker.Stop()

	claimOnce := func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error(ctx, "Redis Stream claim 执行 panic",
					"stream", c.cfg.logicalStream,
					"group", c.cfg.group,
					"panic", r,
				)
			}
		}()
		c.claimPending(ctx, handler, processMu)
	}

	claimOnce()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			claimOnce()
		}
	}
}

// ensureGroup 创建消费组（MKSTREAM）；组已存在时忽略 BUSYGROUP
func (c *consumer) ensureGroup(ctx context.Context) error {
	err := c.client.XGroupCreateMkStream(ctx, c.cfg.streamKey, c.cfg.group, c.cfg.start).Err()
	if err == nil {
		return nil
	}
	if isBusyGroup(err) {
		return nil
	}
	return errors.Wrap(err, "创建消费组失败 stream=%s group=%s", c.cfg.logicalStream, c.cfg.group)
}

// claimPending 通过 XAUTOCLAIM 回收空闲过久的 PEL 消息并交给 handler；
// 仅在调用 Handler 时持有 processMu，XACK 等 Redis 收尾在锁外执行
func (c *consumer) claimPending(ctx context.Context, handler Handler, processMu *sync.Mutex) {
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
		if err := c.processMessages(ctx, handler, messages, processMu); err != nil {
			return
		}
		if next == "0-0" || len(messages) == 0 {
			return
		}
		start = next
	}
}

// processMessages 串行调用 Handler，收集本批待确认 ID，结束后一次批量 XACK。
// HandleDead 先 moveToDead 再纳入 ack 批次，保证死信写入早于确认。
func (c *consumer) processMessages(
	ctx context.Context,
	handler Handler,
	messages []redisv9.XMessage,
	processMu *sync.Mutex,
) error {
	if len(messages) == 0 {
		return nil
	}
	ackIDs := make([]string, 0, len(messages))
	for _, msg := range messages {
		entry := Entry{
			Stream: c.cfg.logicalStream,
			ID:     msg.ID,
			Values: stringifyValues(msg.Values),
		}
		processMu.Lock()
		result := invokeHandler(ctx, handler, entry)
		processMu.Unlock()

		shouldAck, err := c.finalizeHandleResult(ctx, entry, result)
		if err != nil {
			return err
		}
		if shouldAck {
			ackIDs = append(ackIDs, entry.ID)
		}
	}
	return c.ackWithRetry(ctx, ackIDs...)
}

// finalizeHandleResult 按 Handler 返回值做非 XACK 收尾，返回是否应纳入本批确认
func (c *consumer) finalizeHandleResult(ctx context.Context, entry Entry, result HandleResult) (bool, error) {
	switch result {
	case HandleAck:
		return true, nil
	case HandleDead:
		if err := c.moveToDead(ctx, entry); err != nil {
			logger.Error(ctx, "写入 Redis Stream dead 失败，保留在 PEL",
				"stream", entry.Stream,
				"id", entry.ID,
				"error", err,
			)
			return false, nil
		}
		return true, nil
	case HandleRetry:
		logger.Warn(ctx, "Redis Stream 消费返回 Retry，保留在 PEL 并继续",
			"stream", entry.Stream,
			"id", entry.ID,
		)
		return false, nil
	default:
		logger.Warn(ctx, "Redis Stream 未知 HandleResult，按 Retry 保留在 PEL 并继续",
			"stream", entry.Stream,
			"id", entry.ID,
			"result", result,
		)
		return false, nil
	}
}

// ackWithRetry 批量提交 XACK；瞬时失败有限次退避重试，超限或不可恢复错误则放弃并留 PEL
func (c *consumer) ackWithRetry(ctx context.Context, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}
	delay := jitterDelay(ackRetryInitialDelay)
	for attempt := 1; attempt <= ackRetryMaxAttempts; attempt++ {
		err := c.client.XAck(ctx, c.cfg.streamKey, c.cfg.group, ids...).Err()
		if err == nil {
			return nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if isUnrecoverableStreamError(err) {
			logger.Error(ctx, "Redis Stream XACK 不可恢复失败，保留在 PEL",
				"ids", ids,
				"error", err,
			)
			return nil
		}
		if attempt >= ackRetryMaxAttempts {
			logger.Error(ctx, "Redis Stream XACK 重试超限，保留在 PEL",
				"ids", ids,
				"attempts", attempt,
				"error", err,
			)
			return nil
		}
		logger.Warn(ctx, "Redis Stream XACK 失败，准备重试",
			"ids", ids,
			"attempt", attempt,
			"error", err,
		)
		if waitErr := waitForRetry(ctx, delay); waitErr != nil {
			return waitErr
		}
		delay = nextAckRetryDelay(delay)
	}
	return nil
}

// isUnrecoverableStreamError 判断是否为不宜继续重试的 Stream 错误
func isUnrecoverableStreamError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, redisv9.ErrClosed) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "NOGROUP") || strings.Contains(msg, "WRONGTYPE")
}

// nextAckRetryDelay 计算 XACK 专用退避（上限低于读循环重试）
func nextAckRetryDelay(current time.Duration) time.Duration {
	next := current * 2
	if next > ackRetryMaxDelay {
		next = ackRetryMaxDelay
	}
	return jitterDelay(next)
}

// moveToDead 将消息写入死信 stream；框架元数据写入独立字段 DeadMetaField，不覆盖业务字段
func (c *consumer) moveToDead(ctx context.Context, entry Entry) error {
	values := make(map[string]any, len(entry.Values)+1)
	for k, v := range entry.Values {
		values[k] = v
	}
	metaRaw, err := json.Marshal(deadMeta{
		OriginalID: entry.ID,
		Stream:     entry.Stream,
	})
	if err != nil {
		return errors.Wrap(err, "序列化死信 meta 失败")
	}
	values[DeadMetaField] = string(metaRaw)

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

// invokeHandler 调用消费 handler 并捕获 panic，按 HandleRetry 处理
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

// stringifyValues 将 Redis 返回的字段值统一转为 string map
func stringifyValues(values map[string]any) map[string]string {
	out := make(map[string]string, len(values))
	for k, v := range values {
		out[k] = valueToString(v)
	}
	return out
}

// valueToString 将单个字段值转为字符串
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

// isBusyGroup 判断错误是否表示消费组已存在
func isBusyGroup(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "BUSYGROUP")
}

// waitForRetry 等待退避时长，上下文取消时立即返回
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

// jitterDelay 为基准退避时长加上抖动
func jitterDelay(base time.Duration) time.Duration {
	n := consumerRetryJitterMin + rand.IntN(consumerRetryJitterMax-consumerRetryJitterMin+1)
	return base * time.Duration(n) / time.Duration(consumerRetryJitterDenom)
}

// nextRetryDelay 计算下一次指数退避时长并加抖动
func nextRetryDelay(current time.Duration) time.Duration {
	next := current * 2
	if next > consumerRetryMaxDelay {
		next = consumerRetryMaxDelay
	}
	return jitterDelay(next)
}
