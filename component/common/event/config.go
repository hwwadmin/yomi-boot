// Package event 提供消息队列事件配置
//
// @author zwei
package event

import (
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/event/cfg"
	"gitee.com/hq_project/yomi-boot/component/common/roundrobin"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

const (
	defaultEventServiceName          = "yomi-event"
	defaultEventFetchFrequencyMS     = 1000
	defaultEventFetchSize            = 5000
	defaultEventSentThreadCount      = 2
	defaultEventOutboxDlqFrequencyMS = 1000
	defaultEventOutboxDlqSize        = 5000
	defaultEventOutboxDlqMaxRetry    = 30
	defaultEventFetchTimeoutMins     = 5
	defaultEventInboxDlqMaxRetry     = 20
	defaultEventInboxDlqFrequencyMS  = 30000
	defaultEventInboxDlqSize         = 100
	defaultEventCleanupRetentionDays = 30
	defaultEventCleanupFrequencyMS   = 3600000
	defaultEventCleanupBatchSize     = 500
	defaultEventCleanupMaxBatches    = 20
)

// Config 表示消息队列事件配置
type Config struct {
	OutboxEnabled       bool   // OutboxEnabled 是否启用发布侧（写 Outbox + 发件箱后台投递）
	InboxEnabled        bool   // InboxEnabled 是否启用消费侧（Receiver/Inbox）
	ServiceName         string // ServiceName 是归属业务系统名称，未配置时使用 app.name
	FetchFrequencyMS    int    // FetchFrequencyMS 是发件箱抓取频率（毫秒）
	FetchSize           int    // FetchSize 是单次抓取待发件箱事件数量
	SentThreadCount     int    // SentThreadCount 是发件箱 Kafka 发送工作池并发数
	FetchTimeoutMinutes int    // FetchTimeoutMinutes 是发件箱事件停留在 FETCHED 状态的最长分钟数；超过后重置为 PENDING 重新投递，未配置或 <=0 时默认 5 分钟

	// outbox DLQ
	OutboxDlqFrequencyMS int  // OutboxDlqFrequencyMS 是发件箱死信队列重试轮询频率（毫秒）
	OutboxDlqSize        int  // OutboxDlqSize 是单次抓取发件箱死信记录数量
	OutboxDlqMaxRetry    *int // OutboxDlqMaxRetry 是发件箱死信队列最大重试次数，nil 默认 30，0 表示不重试

	// inbox DLQ
	InboxDlqMaxRetry    *int // InboxDlqMaxRetry 是消费失败最大重试次数（不含首次处理），nil 默认 20，0 不重试，-1 无限；Kafka MaxHandlerRetries 须为 -1 或不小于本值，否则 Kafka 会先丢弃消息
	InboxDlqFrequencyMS int  // InboxDlqFrequencyMS 是收件箱死信后台重投频率（毫秒），未配置默认 30000
	InboxDlqSize        int  // InboxDlqSize 是收件箱死信单次重投数量，未配置默认 100

	// outbox cleanup
	OutboxCleanupEnabled     bool // OutboxCleanupEnabled 是否启用发件箱过期数据清理
	OutboxRetentionDays      int  // OutboxRetentionDays 是发送成功记录的保留天数，未配置默认 30
	OutboxCleanupFrequencyMS int  // OutboxCleanupFrequencyMS 是发件箱清理频率（毫秒），未配置默认 3600000
	OutboxCleanupBatchSize   int  // OutboxCleanupBatchSize 是发件箱单批清理数量，未配置默认 500
	OutboxCleanupMaxBatches  int  // OutboxCleanupMaxBatches 是发件箱单轮最大清理批数，未配置默认 20

	// inbox cleanup
	InboxCleanupEnabled     bool // InboxCleanupEnabled 是否启用收件箱过期数据清理
	InboxRetentionDays      int  // InboxRetentionDays 是已消费记录的保留天数，未配置默认 30；必须大于 Kafka 保留期与最大消费滞后
	InboxCleanupFrequencyMS int  // InboxCleanupFrequencyMS 是收件箱清理频率（毫秒），未配置默认 3600000
	InboxCleanupBatchSize   int  // InboxCleanupBatchSize 是收件箱单批清理数量，未配置默认 500
	InboxCleanupMaxBatches  int  // InboxCleanupMaxBatches 是收件箱单轮最大清理批数，未配置默认 20
}

// AnyEnabled 返回发布侧或消费侧是否任一启用
func (c Config) AnyEnabled() bool {
	return c.OutboxEnabled || c.InboxEnabled
}

// ServiceNameOrDefault 返回归属业务系统名称
func (c Config) ServiceNameOrDefault(appName string) string {
	if name := strings.TrimSpace(c.ServiceName); name != "" {
		return name
	}
	if name := strings.TrimSpace(appName); name != "" {
		return name
	}
	return defaultEventServiceName
}

// FetchFrequencyOrDefault 返回发件箱抓取频率
func (c Config) FetchFrequencyOrDefault() time.Duration {
	if c.FetchFrequencyMS <= 0 {
		return time.Duration(defaultEventFetchFrequencyMS) * time.Millisecond
	}
	return time.Duration(c.FetchFrequencyMS) * time.Millisecond
}

// FetchSizeOrDefault 返回发件箱抓取数量
func (c Config) FetchSizeOrDefault() int {
	if c.FetchSize <= 0 {
		return defaultEventFetchSize
	}
	return c.FetchSize
}

// SentThreadCountOrDefault 返回发件箱发送线程数
func (c Config) SentThreadCountOrDefault() int {
	if c.SentThreadCount <= 0 {
		return defaultEventSentThreadCount
	}
	return c.SentThreadCount
}

// OutboxDlqFrequencyOrDefault 返回发件箱死信队列处理频率
func (c Config) OutboxDlqFrequencyOrDefault() time.Duration {
	if c.OutboxDlqFrequencyMS <= 0 {
		return time.Duration(defaultEventOutboxDlqFrequencyMS) * time.Millisecond
	}
	return time.Duration(c.OutboxDlqFrequencyMS) * time.Millisecond
}

// OutboxDlqSizeOrDefault 返回发件箱死信队列抓取数量
func (c Config) OutboxDlqSizeOrDefault() int {
	if c.OutboxDlqSize <= 0 {
		return defaultEventOutboxDlqSize
	}
	return c.OutboxDlqSize
}

// OutboxDlqMaxRetryOrDefault 返回发件箱死信队列最大重试次数
func (c Config) OutboxDlqMaxRetryOrDefault() int {
	if c.OutboxDlqMaxRetry == nil {
		return defaultEventOutboxDlqMaxRetry
	}
	return *c.OutboxDlqMaxRetry
}

// FetchTimeoutOrDefault 返回 FETCHED 事件超时重置为 PENDING 的时长
func (c Config) FetchTimeoutOrDefault() time.Duration {
	if c.FetchTimeoutMinutes <= 0 {
		return time.Duration(defaultEventFetchTimeoutMins) * time.Minute
	}
	return time.Duration(c.FetchTimeoutMinutes) * time.Minute
}

// InboxDlqMaxRetryOrDefault 返回消费失败最大重试次数
func (c Config) InboxDlqMaxRetryOrDefault() int {
	if c.InboxDlqMaxRetry == nil {
		return defaultEventInboxDlqMaxRetry
	}
	return *c.InboxDlqMaxRetry
}

// InboxDlqFrequencyOrDefault 返回收件箱死信后台重投频率
func (c Config) InboxDlqFrequencyOrDefault() time.Duration {
	if c.InboxDlqFrequencyMS <= 0 {
		return time.Duration(defaultEventInboxDlqFrequencyMS) * time.Millisecond
	}
	return time.Duration(c.InboxDlqFrequencyMS) * time.Millisecond
}

// InboxDlqSizeOrDefault 返回收件箱死信单次重投数量
func (c Config) InboxDlqSizeOrDefault() int {
	if c.InboxDlqSize <= 0 {
		return defaultEventInboxDlqSize
	}
	return c.InboxDlqSize
}

// cleanupRuntime 将清理配置转换为运行时配置
func cleanupRuntime(enabled bool, retentionDays, frequencyMS, batchSize, maxBatches int) cfg.CleanupRuntime {
	if retentionDays <= 0 {
		retentionDays = defaultEventCleanupRetentionDays
	}
	if frequencyMS <= 0 {
		frequencyMS = defaultEventCleanupFrequencyMS
	}
	if batchSize <= 0 {
		batchSize = defaultEventCleanupBatchSize
	}
	if maxBatches <= 0 {
		maxBatches = defaultEventCleanupMaxBatches
	}
	return cfg.CleanupRuntime{
		Enabled:    enabled,
		Retention:  time.Duration(retentionDays) * 24 * time.Hour,
		Frequency:  time.Duration(frequencyMS) * time.Millisecond,
		BatchSize:  batchSize,
		MaxBatches: maxBatches,
	}
}

// validateCleanupConfig 校验单组清理配置
func validateCleanupConfig(
	prefix string,
	minFrequencyMS int,
	retentionDays int,
	frequencyMS int,
	batchSize int,
	maxBatches int,
) error {
	if retentionDays < 0 {
		return errors.Newf("%s.retention_days 不能小于 0", prefix)
	}
	if frequencyMS < 0 {
		return errors.Newf("%s.frequency_ms 不能小于 0", prefix)
	}
	if frequencyMS > 0 && frequencyMS < minFrequencyMS {
		return errors.Newf("%s.frequency_ms 必须大于等于 %s", prefix, roundrobin.MinInterval.String())
	}
	if batchSize < 0 {
		return errors.Newf("%s.batch_size 不能小于 0", prefix)
	}
	if maxBatches < 0 {
		return errors.Newf("%s.max_batches 不能小于 0", prefix)
	}
	return nil
}

// Validate 校验消息队列事件配置（发布侧或消费侧启用时校验）
func (c Config) Validate() error {
	if !c.AnyEnabled() {
		return nil
	}
	minFrequencyMS := int(roundrobin.MinInterval / time.Millisecond)
	if c.FetchFrequencyMS < 0 {
		return errors.New("fetch_frequency_ms 不能小于 0")
	}
	if c.FetchFrequencyMS > 0 && c.FetchFrequencyMS < minFrequencyMS {
		return errors.Newf("fetch_frequency_ms 必须大于等于 %s", roundrobin.MinInterval.String())
	}
	if c.FetchSize < 0 {
		return errors.New("fetch_size 不能小于 0")
	}
	if c.SentThreadCount < 0 {
		return errors.New("sent_thread_count 不能小于 0")
	}
	if c.OutboxDlqFrequencyMS < 0 {
		return errors.New("outbox_dlq_frequency_ms 不能小于 0")
	}
	if c.OutboxDlqFrequencyMS > 0 && c.OutboxDlqFrequencyMS < minFrequencyMS {
		return errors.Newf("outbox_dlq_frequency_ms 必须大于等于 %s", roundrobin.MinInterval.String())
	}
	if c.OutboxDlqSize < 0 {
		return errors.New("outbox_dlq_size 不能小于 0")
	}
	if c.OutboxDlqMaxRetry != nil && *c.OutboxDlqMaxRetry < 0 {
		return errors.New("outbox_dlq_max_retry 不能小于 0")
	}
	if c.FetchTimeoutMinutes < 0 {
		return errors.New("fetch_timeout_minutes 不能小于 0")
	}
	if c.InboxDlqMaxRetry != nil && *c.InboxDlqMaxRetry < -1 {
		return errors.New("inbox_dlq_max_retry 不能小于 -1")
	}
	if c.InboxDlqFrequencyMS < 0 {
		return errors.New("inbox_dlq_frequency_ms 不能小于 0")
	}
	if c.InboxDlqFrequencyMS > 0 && c.InboxDlqFrequencyMS < minFrequencyMS {
		return errors.Newf("inbox_dlq_frequency_ms 必须大于等于 %s", roundrobin.MinInterval.String())
	}
	if c.InboxDlqSize < 0 {
		return errors.New("inbox_dlq_size 不能小于 0")
	}
	if err := validateCleanupConfig(
		"outbox_cleanup",
		minFrequencyMS,
		c.OutboxRetentionDays,
		c.OutboxCleanupFrequencyMS,
		c.OutboxCleanupBatchSize,
		c.OutboxCleanupMaxBatches,
	); err != nil {
		return err
	}
	if err := validateCleanupConfig(
		"inbox_cleanup",
		minFrequencyMS,
		c.InboxRetentionDays,
		c.InboxCleanupFrequencyMS,
		c.InboxCleanupBatchSize,
		c.InboxCleanupMaxBatches,
	); err != nil {
		return err
	}
	return nil
}

// runtimeFrom 从应用配置解析事件运行时配置
func runtimeFrom(eventCfg Config, appName string) cfg.Runtime {
	return cfg.Runtime{
		ServiceName:        eventCfg.ServiceNameOrDefault(appName),
		FetchFrequency:     eventCfg.FetchFrequencyOrDefault(),
		FetchSize:          eventCfg.FetchSizeOrDefault(),
		SentThreadCount:    eventCfg.SentThreadCountOrDefault(),
		OutboxDlqFrequency: eventCfg.OutboxDlqFrequencyOrDefault(),
		OutboxDlqSize:      eventCfg.OutboxDlqSizeOrDefault(),
		OutboxDlqMaxRetry:  eventCfg.OutboxDlqMaxRetryOrDefault(),
		FetchTimeout:       eventCfg.FetchTimeoutOrDefault(),
		InboxDlqMaxRetry:   eventCfg.InboxDlqMaxRetryOrDefault(),
		InboxDlqFrequency:  eventCfg.InboxDlqFrequencyOrDefault(),
		InboxDlqSize:       eventCfg.InboxDlqSizeOrDefault(),
		OutboxCleanup: cleanupRuntime(
			eventCfg.OutboxCleanupEnabled,
			eventCfg.OutboxRetentionDays,
			eventCfg.OutboxCleanupFrequencyMS,
			eventCfg.OutboxCleanupBatchSize,
			eventCfg.OutboxCleanupMaxBatches,
		),
		InboxCleanup: cleanupRuntime(
			eventCfg.InboxCleanupEnabled,
			eventCfg.InboxRetentionDays,
			eventCfg.InboxCleanupFrequencyMS,
			eventCfg.InboxCleanupBatchSize,
			eventCfg.InboxCleanupMaxBatches,
		),
		OutboxEnabled: eventCfg.OutboxEnabled,
		InboxEnabled:  eventCfg.InboxEnabled,
	}
}
