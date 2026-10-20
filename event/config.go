// Package event 提供消息队列事件配置
//
// @author zwei
package event

import (
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/config"
	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/event/cfg"
	"gitee.com/hq_project/yomi-boot/roundrobin"
)

const (
	// SectionKey YAML 根 key
	SectionKey = "event"

	defaultEventServiceName         = "yomi-event"
	defaultEventFetchFrequencyMS    = 1000
	defaultEventFetchSize           = 5000
	defaultEventSentThreadCount     = 2
	defaultEventDlqFrequencyMS      = 1000
	defaultEventDlqSize             = 5000
	defaultEventDlqMaxRetry         = 30
	defaultEventFetchTimeoutMins    = 5
	defaultEventConsumerMaxRetry    = 20
	defaultEventInboxDlqFrequencyMS = 30000
	defaultEventInboxDlqSize        = 100
)

// Config 表示消息队列事件配置
type Config struct {
	PublisherEnabled    *bool  `mapstructure:"publisher_enabled"`      // PublisherEnabled 是否启用发布侧（写 Outbox + 发件箱后台投递），未配置默认 false
	ConsumerEnabled     *bool  `mapstructure:"consumer_enabled"`       // ConsumerEnabled 是否启用消费侧（Receiver/Inbox），未配置默认 false
	ServiceName         string `mapstructure:"service_name"`           // ServiceName 是归属业务系统名称，未配置时使用 app.name
	FetchFrequencyMS    int    `mapstructure:"fetch_frequency_ms"`     // FetchFrequencyMS 是发件箱抓取频率（毫秒）
	FetchSize           int    `mapstructure:"fetch_size"`             // FetchSize 是单次抓取待发件箱事件数量
	SentThreadCount     int    `mapstructure:"sent_thread_count"`      // SentThreadCount 是发件箱 Kafka 发送工作池并发数
	DlqFrequencyMS      int    `mapstructure:"dlq_frequency_ms"`       // DlqFrequencyMS 是死信队列重试轮询频率（毫秒）
	DlqSize             int    `mapstructure:"dlq_size"`               // DlqSize 是单次抓取死信记录数量
	DlqMaxRetry         *int   `mapstructure:"dlq_max_retry"`          // DlqMaxRetry 是死信队列最大重试次数，nil 默认 30，0 表示不重试
	FetchTimeoutMinutes int    `mapstructure:"fetch_timeout_minutes"`  // FetchTimeoutMinutes 是 FETCHED 状态超时回收时间（分钟）
	ConsumerMaxRetry    *int   `mapstructure:"consumer_max_retry"`     // ConsumerMaxRetry 是消费失败最大重试次数（不含首次处理），nil 默认 20，0 不重试，-1 无限；Kafka MaxHandlerRetries 须为 -1 或不小于本值，否则 Kafka 会先丢弃消息
	InboxDlqFrequencyMS int    `mapstructure:"inbox_dlq_frequency_ms"` // InboxDlqFrequencyMS 是收件箱死信后台重投频率（毫秒），未配置默认 30000
	InboxDlqSize        int    `mapstructure:"inbox_dlq_size"`         // InboxDlqSize 是收件箱死信单次重投数量，未配置默认 100
}

// PublisherEnabledOrDefault 返回是否启用发布侧，未配置默认 false
func (c Config) PublisherEnabledOrDefault() bool {
	return config.BoolOrDefault(c.PublisherEnabled)
}

// ConsumerEnabledOrDefault 返回是否启用消费侧，未配置默认 false
func (c Config) ConsumerEnabledOrDefault() bool {
	return config.BoolOrDefault(c.ConsumerEnabled)
}

// AnyEnabledOrDefault 返回发布侧或消费侧是否任一启用
func (c Config) AnyEnabledOrDefault() bool {
	return c.PublisherEnabledOrDefault() || c.ConsumerEnabledOrDefault()
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

// DlqFrequencyOrDefault 返回 DLQ 处理频率
func (c Config) DlqFrequencyOrDefault() time.Duration {
	if c.DlqFrequencyMS <= 0 {
		return time.Duration(defaultEventDlqFrequencyMS) * time.Millisecond
	}
	return time.Duration(c.DlqFrequencyMS) * time.Millisecond
}

// DlqSizeOrDefault 返回 DLQ 抓取数量
func (c Config) DlqSizeOrDefault() int {
	if c.DlqSize <= 0 {
		return defaultEventDlqSize
	}
	return c.DlqSize
}

// DlqMaxRetryOrDefault 返回 DLQ 最大重试次数
func (c Config) DlqMaxRetryOrDefault() int {
	if c.DlqMaxRetry == nil {
		return defaultEventDlqMaxRetry
	}
	return *c.DlqMaxRetry
}

// FetchTimeoutOrDefault 返回 FETCHED 状态超时时间
func (c Config) FetchTimeoutOrDefault() time.Duration {
	if c.FetchTimeoutMinutes <= 0 {
		return time.Duration(defaultEventFetchTimeoutMins) * time.Minute
	}
	return time.Duration(c.FetchTimeoutMinutes) * time.Minute
}

// ConsumerMaxRetryOrDefault 返回消费失败最大重试次数
func (c Config) ConsumerMaxRetryOrDefault() int {
	if c.ConsumerMaxRetry == nil {
		return defaultEventConsumerMaxRetry
	}
	return *c.ConsumerMaxRetry
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

// Validate 校验消息队列事件配置（发布侧或消费侧启用时校验）
func (c Config) Validate() error {
	if !c.AnyEnabledOrDefault() {
		return nil
	}
	minFrequencyMS := int(roundrobin.MinInterval / time.Millisecond)
	if c.FetchFrequencyMS < 0 {
		return errors.New("event.fetch_frequency_ms 不能小于 0")
	}
	if c.FetchFrequencyMS > 0 && c.FetchFrequencyMS < minFrequencyMS {
		return errors.Newf("event.fetch_frequency_ms 必须大于等于 %s", roundrobin.MinInterval.String())
	}
	if c.FetchSize < 0 {
		return errors.New("event.fetch_size 不能小于 0")
	}
	if c.SentThreadCount < 0 {
		return errors.New("event.sent_thread_count 不能小于 0")
	}
	if c.DlqFrequencyMS < 0 {
		return errors.New("event.dlq_frequency_ms 不能小于 0")
	}
	if c.DlqFrequencyMS > 0 && c.DlqFrequencyMS < minFrequencyMS {
		return errors.Newf("event.dlq_frequency_ms 必须大于等于 %s", roundrobin.MinInterval.String())
	}
	if c.DlqSize < 0 {
		return errors.New("event.dlq_size 不能小于 0")
	}
	if c.DlqMaxRetry != nil && *c.DlqMaxRetry < 0 {
		return errors.New("event.dlq_max_retry 不能小于 0")
	}
	if c.FetchTimeoutMinutes < 0 {
		return errors.New("event.fetch_timeout_minutes 不能小于 0")
	}
	if c.ConsumerMaxRetry != nil && *c.ConsumerMaxRetry < -1 {
		return errors.New("event.consumer_max_retry 不能小于 -1")
	}
	if c.InboxDlqFrequencyMS < 0 {
		return errors.New("event.inbox_dlq_frequency_ms 不能小于 0")
	}
	if c.InboxDlqFrequencyMS > 0 && c.InboxDlqFrequencyMS < minFrequencyMS {
		return errors.Newf("event.inbox_dlq_frequency_ms 必须大于等于 %s", roundrobin.MinInterval.String())
	}
	if c.InboxDlqSize < 0 {
		return errors.New("event.inbox_dlq_size 不能小于 0")
	}
	return nil
}

// ConfigSection 返回 event 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &Config{}, func(c *Config, _ *config.Document) error {
		return c.Validate()
	})
}

// runtimeFrom 从应用配置解析事件运行时配置
func runtimeFrom(eventCfg Config, appName string) cfg.Runtime {
	return cfg.Runtime{
		ServiceName:       eventCfg.ServiceNameOrDefault(appName),
		FetchFrequency:    eventCfg.FetchFrequencyOrDefault(),
		FetchSize:         eventCfg.FetchSizeOrDefault(),
		SentThreadCount:   eventCfg.SentThreadCountOrDefault(),
		DlqFrequency:      eventCfg.DlqFrequencyOrDefault(),
		DlqSize:           eventCfg.DlqSizeOrDefault(),
		DlqMaxRetry:       eventCfg.DlqMaxRetryOrDefault(),
		FetchTimeout:      eventCfg.FetchTimeoutOrDefault(),
		ConsumerMaxRetry:  eventCfg.ConsumerMaxRetryOrDefault(),
		InboxDlqFrequency: eventCfg.InboxDlqFrequencyOrDefault(),
		InboxDlqSize:      eventCfg.InboxDlqSizeOrDefault(),
		PublisherEnabled:  eventCfg.PublisherEnabledOrDefault(),
		ConsumerEnabled:   eventCfg.ConsumerEnabledOrDefault(),
	}
}
