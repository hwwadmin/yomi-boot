// HTTP 访问记录消费端：处理函数注册与消费生命周期。

package httprecordplugin

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/hwwadmin/yomi-boot/component/common/httprecord"
	"github.com/hwwadmin/yomi-boot/component/core/errors"
	"github.com/hwwadmin/yomi-boot/component/core/logger"
	"github.com/hwwadmin/yomi-boot/component/core/traceid"
	"github.com/hwwadmin/yomi-boot/plugin"
	appctxplugin "github.com/hwwadmin/yomi-boot/plugin/builtin/appctx"
)

// MustRegisterHandler 注册 HTTP 访问记录消费处理函数，须在 OnReady 之前调用（通常在 wire 中）。
// handler 为 nil、重复注册或插件未注册时 panic；handler 会被多个消费协程/分区并发调用，
// 须自行保证并发安全。
func (p *HttpRecord) MustRegisterHandler(handler httprecord.RecordHandler) {
	if p == nil {
		panic(errors.New("http_record: 插件未注册，无法注册 RecordHandler"))
	}
	if handler == nil {
		panic(errors.New("http_record: RecordHandler 不能为空"))
	}

	p.handlerMu.Lock()
	defer p.handlerMu.Unlock()
	if p.handler != nil {
		panic(errors.New("http_record: RecordHandler 已注册，禁止重复注册"))
	}
	p.handler = handler
}

// OnReady 按 provider 启动消费者；已启用消费但未注册处理函数时启动失败
func (p *HttpRecord) OnReady(context.Context) error {
	if p == nil || p.service == nil {
		return nil
	}
	if !p.shouldConsume() {
		if p.registeredHandler() != nil {
			logger.WarnWithoutCtx("HTTP 访问记录已注册处理函数，但当前未启用消费，已忽略", "provider", p.provider)
		}
		return nil
	}

	handler := p.registeredHandler()
	if handler == nil {
		return errors.New("http_record: 已启用消费但未注册 RecordHandler，请在 wire 中调用 httprecordplugin.From(slot).MustRegisterHandler")
	}
	handler = withTraceparent(handler)

	var err error
	switch p.provider {
	case httprecord.ProviderMemory:
		err = httprecord.StartMemoryRecordConsumer(p.runtimeCtx, p.memorySvc,
			httprecord.MemoryRecordConsumerConfig{Workers: p.cfg.Memory.WorkersOrDefault()}, handler)
	case httprecord.ProviderKafka:
		err = httprecord.StartKafkaRecordConsumer(p.runtimeCtx, p.kafkaConsumer, httprecord.KafkaConsumerID,
			httprecord.KafkaRecordConsumerConfig{
				Topics:     []string{p.cfg.KafkaTopicOrDefault()},
				GroupID:    p.consumerGroup,
				MaxRetries: p.cfg.Consumer.MaxRetry,
			}, handler)
	case httprecord.ProviderRedisStream:
		err = httprecord.StartRedisStreamRecordConsumer(p.runtimeCtx, p.streamConsumer, httprecord.RedisStreamConsumerID,
			httprecord.RedisStreamRecordConsumerConfig{
				Stream:   p.cfg.StreamOrDefault(),
				Group:    p.consumerGroup,
				Consumer: p.consumerName,
			}, handler)
	default:
		return errors.Newf("http_record: 不支持的 provider: %s", p.provider)
	}
	if err != nil {
		return errors.Wrap(err, "http_record: 启动消费者失败")
	}
	logger.InfoWithoutCtx("HTTP 访问记录消费者已启动", "provider", p.provider)
	return nil
}

// shouldConsume 返回本进程是否承担消费角色：memory 恒消费，kafka / redis_stream 由 consumer.enabled 控制
func (p *HttpRecord) shouldConsume() bool {
	return p.provider == httprecord.ProviderMemory || p.cfg.Consumer.EnabledOrDefault()
}

// registeredHandler 返回已注册的处理函数；未注册时为 nil
func (p *HttpRecord) registeredHandler() httprecord.RecordHandler {
	p.handlerMu.Lock()
	defer p.handlerMu.Unlock()
	return p.handler
}

// withTraceparent 把记录中的 TraceParent 注入消费上下文，便于业务日志串联链路
func withTraceparent(handler httprecord.RecordHandler) httprecord.RecordHandler {
	return func(ctx context.Context, record *httprecord.HttpRecord) httprecord.RecordHandleResult {
		if record != nil && record.TraceParent != "" {
			ctx = traceid.ContextWithTraceparent(ctx, record.TraceParent)
		}
		return handler(ctx, record)
	}
}

// appName 返回应用名；appctx 未就绪时返回空串
func appName(slot *plugin.PluginSlot) string {
	if app := appctxplugin.From(slot).AppCtx(); app != nil {
		return app.Name()
	}
	return ""
}

// resolveConsumerGroupID 返回消费组：显式配置优先，未配置按 应用名-httprecord 派生
func resolveConsumerGroupID(configured, appName string) string {
	if groupID := strings.TrimSpace(configured); groupID != "" {
		return groupID
	}
	if name := strings.TrimSpace(appName); name != "" {
		return name + "-httprecord"
	}
	return "httprecord"
}

// redisConsumerName 生成同组内唯一的消费者名：应用名-主机名-进程号
func redisConsumerName(appName string) string {
	name := strings.TrimSpace(appName)
	if name == "" {
		name = "httprecord"
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	return fmt.Sprintf("%s-%s-%d", name, host, os.Getpid())
}
