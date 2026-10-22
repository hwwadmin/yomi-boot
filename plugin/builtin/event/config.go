package eventplugin

import (
	"gitee.com/hq_project/yomi-boot/component/common/event"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// SectionKey YAML 根 key
const SectionKey = "event"

// BootConfig 表示 event 配置段的 YAML 绑定
type BootConfig struct {
	OutboxEnabled       *bool  `mapstructure:"outbox_enabled"`        // OutboxEnabled 是否启用发布侧（写 Outbox + 发件箱后台投递），未配置默认 false
	InboxEnabled        *bool  `mapstructure:"inbox_enabled"`         // InboxEnabled 是否启用消费侧（Receiver/Inbox），未配置默认 false
	ServiceName         string `mapstructure:"service_name"`          // ServiceName 是归属业务系统名称，未配置时使用 app.name
	FetchFrequencyMS    int    `mapstructure:"fetch_frequency_ms"`    // FetchFrequencyMS 是发件箱抓取频率（毫秒）
	FetchSize           int    `mapstructure:"fetch_size"`            // FetchSize 是单次抓取待发件箱事件数量
	SentThreadCount     int    `mapstructure:"sent_thread_count"`     // SentThreadCount 是发件箱 Kafka 发送工作池并发数
	FetchTimeoutMinutes int    `mapstructure:"fetch_timeout_minutes"` // FetchTimeoutMinutes 是发件箱事件停留在 FETCHED 状态的最长分钟数；超过后重置为 PENDING 重新投递，未配置或 <=0 时默认 5 分钟

	// outbox DLQ
	OutboxDlqFrequencyMS int  `mapstructure:"outbox_dlq_frequency_ms"` // OutboxDlqFrequencyMS 是发件箱死信队列重试轮询频率（毫秒）
	OutboxDlqSize        int  `mapstructure:"outbox_dlq_size"`         // OutboxDlqSize 是单次抓取发件箱死信记录数量
	OutboxDlqMaxRetry    *int `mapstructure:"outbox_dlq_max_retry"`    // OutboxDlqMaxRetry 是发件箱死信队列最大重试次数，nil 默认 30，0 表示不重试

	// inbox DLQ
	InboxDlqMaxRetry    *int `mapstructure:"inbox_dlq_max_retry"`    // InboxDlqMaxRetry 是消费失败最大重试次数（不含首次处理），nil 默认 20，0 不重试，-1 无限；Kafka MaxHandlerRetries 须为 -1 或不小于本值，否则 Kafka 会先丢弃消息
	InboxDlqFrequencyMS int  `mapstructure:"inbox_dlq_frequency_ms"` // InboxDlqFrequencyMS 是收件箱死信后台重投频率（毫秒），未配置默认 30000
	InboxDlqSize        int  `mapstructure:"inbox_dlq_size"`         // InboxDlqSize 是收件箱死信单次重投数量，未配置默认 100

	// outbox cleanup
	OutboxCleanupEnabled     *bool `mapstructure:"outbox_cleanup_enabled"`      // OutboxCleanupEnabled 是否启用发件箱过期数据清理，未配置默认 false
	OutboxRetentionDays      int   `mapstructure:"outbox_retention_days"`       // OutboxRetentionDays 是发送成功记录的保留天数，未配置默认 30
	OutboxCleanupFrequencyMS int   `mapstructure:"outbox_cleanup_frequency_ms"` // OutboxCleanupFrequencyMS 是发件箱清理频率（毫秒），未配置默认 3600000
	OutboxCleanupBatchSize   int   `mapstructure:"outbox_cleanup_batch_size"`   // OutboxCleanupBatchSize 是发件箱单批清理数量，未配置默认 500
	OutboxCleanupMaxBatches  int   `mapstructure:"outbox_cleanup_max_batches"`  // OutboxCleanupMaxBatches 是发件箱单轮最大清理批数，未配置默认 20

	// inbox cleanup
	InboxCleanupEnabled     *bool `mapstructure:"inbox_cleanup_enabled"`      // InboxCleanupEnabled 是否启用收件箱过期数据清理，未配置默认 false
	InboxRetentionDays      int   `mapstructure:"inbox_retention_days"`       // InboxRetentionDays 是已消费记录的保留天数，未配置默认 30；必须大于 Kafka 保留期与最大消费滞后
	InboxCleanupFrequencyMS int   `mapstructure:"inbox_cleanup_frequency_ms"` // InboxCleanupFrequencyMS 是收件箱清理频率（毫秒），未配置默认 3600000
	InboxCleanupBatchSize   int   `mapstructure:"inbox_cleanup_batch_size"`   // InboxCleanupBatchSize 是收件箱单批清理数量，未配置默认 500
	InboxCleanupMaxBatches  int   `mapstructure:"inbox_cleanup_max_batches"`  // InboxCleanupMaxBatches 是收件箱单轮最大清理批数，未配置默认 20
}

// OutboxEnabledOrDefault 返回是否启用发布侧，未配置默认 false
func (c BootConfig) OutboxEnabledOrDefault() bool {
	return c.OutboxEnabled != nil && *c.OutboxEnabled
}

// InboxEnabledOrDefault 返回是否启用消费侧，未配置默认 false
func (c BootConfig) InboxEnabledOrDefault() bool {
	return c.InboxEnabled != nil && *c.InboxEnabled
}

// AnyEnabledOrDefault 返回发布侧或消费侧是否任一启用
func (c BootConfig) AnyEnabledOrDefault() bool {
	return c.OutboxEnabledOrDefault() || c.InboxEnabledOrDefault()
}

// ToConfig 转换为运行时配置
func (c BootConfig) ToConfig() event.Config {
	return event.Config{
		OutboxEnabled:            c.OutboxEnabledOrDefault(),
		InboxEnabled:             c.InboxEnabledOrDefault(),
		ServiceName:              c.ServiceName,
		FetchFrequencyMS:         c.FetchFrequencyMS,
		FetchSize:                c.FetchSize,
		SentThreadCount:          c.SentThreadCount,
		FetchTimeoutMinutes:      c.FetchTimeoutMinutes,
		OutboxDlqFrequencyMS:     c.OutboxDlqFrequencyMS,
		OutboxDlqSize:            c.OutboxDlqSize,
		OutboxDlqMaxRetry:        c.OutboxDlqMaxRetry,
		InboxDlqMaxRetry:         c.InboxDlqMaxRetry,
		InboxDlqFrequencyMS:      c.InboxDlqFrequencyMS,
		InboxDlqSize:             c.InboxDlqSize,
		OutboxCleanupEnabled:     c.OutboxCleanupEnabled != nil && *c.OutboxCleanupEnabled,
		OutboxRetentionDays:      c.OutboxRetentionDays,
		OutboxCleanupFrequencyMS: c.OutboxCleanupFrequencyMS,
		OutboxCleanupBatchSize:   c.OutboxCleanupBatchSize,
		OutboxCleanupMaxBatches:  c.OutboxCleanupMaxBatches,
		InboxCleanupEnabled:      c.InboxCleanupEnabled != nil && *c.InboxCleanupEnabled,
		InboxRetentionDays:       c.InboxRetentionDays,
		InboxCleanupFrequencyMS:  c.InboxCleanupFrequencyMS,
		InboxCleanupBatchSize:    c.InboxCleanupBatchSize,
		InboxCleanupMaxBatches:   c.InboxCleanupMaxBatches,
	}
}

// Validate 校验 event 配置段语义（发布侧或消费侧启用时校验）
func (c BootConfig) Validate() error {
	if !c.AnyEnabledOrDefault() {
		return nil
	}
	if err := c.ToConfig().Validate(); err != nil {
		return errors.Wrap(err, "%s", SectionKey)
	}
	return nil
}

// ConfigSection 返回 event 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &BootConfig{}, func(c *BootConfig, _ *config.Config) error {
		return c.Validate()
	})
}
