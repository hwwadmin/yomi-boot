// Package bootstrap 单元测试
//
// @author zwei
package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/common/appctx"
	"gitee.com/hq_project/yomi-boot/component/common/httprecord"
	"gitee.com/hq_project/yomi-boot/component/common/router"
	"gitee.com/hq_project/yomi-boot/component/common/token"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/cors"
	appctxplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/appctx"
	httprecordplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/httprecord"
	loggerplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/logger"
	redisratelimitplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/redisratelimit"
	tokenplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/token"

	yomikafka "gitee.com/hq_project/yomi-boot/component/common/kafka"
)

// ======================== mock 实现 ========================

// mockTokenService 用于测试的令牌服务实现
type mockTokenService struct{}

// Issue 实现 Issue 接口（测试桩）
func (m *mockTokenService) Issue(_ context.Context, _ *token.TokenSession) (string, error) {
	return "", nil
}

// Validate 实现 Validate 接口（测试桩）
func (m *mockTokenService) Validate(_ context.Context, _ string) (*token.TokenSession, error) {
	return nil, nil
}

// Revoke 实现 Revoke 接口（测试桩）
func (m *mockTokenService) Revoke(_ context.Context, _ string) error {
	return nil
}

// RevokeByUserID 实现 RevokeByUserID 接口（测试桩）
func (m *mockTokenService) RevokeByUserID(_ context.Context, _ int64) error {
	return nil
}

var _ token.TokenService = (*mockTokenService)(nil)

// mockKafkaPublisher 用于测试的 Kafka 生产者实现
type mockKafkaPublisher struct{}

// Publish 实现 Publish 接口（测试桩）
func (m *mockKafkaPublisher) Publish(_ context.Context, _ yomikafka.PublishRequest) error {
	return nil
}

// PublishBatch 实现 PublishBatch 接口（测试桩）
func (m *mockKafkaPublisher) PublishBatch(_ context.Context, _ []yomikafka.PublishRequest) error {
	return nil
}

// PublishJSON 实现 PublishJSON 接口（测试桩）
func (m *mockKafkaPublisher) PublishJSON(_ context.Context, _ string, _ string, _ any) error {
	return nil
}

// PublishAsync 实现 PublishAsync 接口（测试桩）
func (m *mockKafkaPublisher) PublishAsync(_ context.Context, _ yomikafka.PublishRequest) error {
	return nil
}

// PublishBatchAsync 实现 PublishBatchAsync 接口（测试桩）
func (m *mockKafkaPublisher) PublishBatchAsync(_ context.Context, _ []yomikafka.PublishRequest) error {
	return nil
}

// PublishJSONAsync 实现 PublishJSONAsync 接口（测试桩）
func (m *mockKafkaPublisher) PublishJSONAsync(_ context.Context, _ string, _ string, _ any) error {
	return nil
}

// Close 实现 Close 接口（测试桩）
func (m *mockKafkaPublisher) Close() error { return nil }

var _ yomikafka.PublisherService = (*mockKafkaPublisher)(nil)

// mockKafkaConsumer 用于测试的 Kafka 消费者实现
type mockKafkaConsumer struct{}

// Start 实现 Start 接口（测试桩）
func (m *mockKafkaConsumer) Start(_ context.Context, _ string, _ yomikafka.ConsumerConfig, _ yomikafka.ConsumeHandler) error {
	return nil
}

// Stop 实现 Stop 接口（测试桩）
func (m *mockKafkaConsumer) Stop(_ string) error { return nil }

// StopAll 实现 StopAll 接口（测试桩）
func (m *mockKafkaConsumer) StopAll() error { return nil }

// Close 实现 Close 接口（测试桩）
func (m *mockKafkaConsumer) Close() error { return nil }

var _ yomikafka.ConsumerService = (*mockKafkaConsumer)(nil)

// middlewareTestRegistry 返回中间件单测用配置注册表
func middlewareTestRegistry() *config.Registry {
	return config.NewRegistry().Register(
		appctxplugin.ConfigSection(),
		router.ConfigSection(),
		loggerplugin.ConfigSection(),
		cors.ConfigSection(),
		tokenplugin.ConfigSection(),
		httprecordplugin.ConfigSection(),
		redisratelimitplugin.ConfigSection(),
	)
}

// loadMiddlewareTestDoc 写入并加载中间件单测配置文档
func loadMiddlewareTestDoc(t *testing.T, authEnabled bool) *config.Document {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	authBlock := "auth: {}"
	if authEnabled {
		authBlock = `auth:
  enabled: true
  token_ttl_minutes: 60
  store_prefix: test`
	}
	content := `
app:
  name: test
  env: test
  version: v1
http:
  port: 8080
  router_prefix: /api
log:
  rotation:
    max_size_mb: 50
    retention_days: 3
    max_backups: 30
    max_total_mb: 2048
cors: {}
` + authBlock + `
http_record: {}
ratelimit: {}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写临时配置失败: %v", err)
	}
	doc, err := config.LoadDocument(path, "YOMI_TEST", middlewareTestRegistry())
	if err != nil {
		t.Fatalf("加载临时配置失败: %v", err)
	}
	return doc
}

// ======================== 测试用例 ========================

// TestInfra_CleanupNilReceiver 测试 nil Infra 调用 Cleanup 不崩溃
func TestInfra_CleanupNilReceiver(t *testing.T) {
	var infra *Infra
	infra.Cleanup()
}

// TestDefaultMiddlewares_AlwaysIncludesAuth 测试中间件链始终包含令牌鉴权位（是否生效由 auth.enabled 映射到组件内开关）
func TestDefaultMiddlewares_AlwaysIncludesAuth(t *testing.T) {
	docEnabled := loadMiddlewareTestDoc(t, true)
	infraEnabled := &Infra{
		Doc:               docEnabled,
		TokenService:      &mockTokenService{},
		HttpRecordService: httprecord.NewNopService(),
		AppCtx:            appctx.NewAppCtx("test", "test", "v1"),
	}
	middlewaresEnabled := DefaultMiddlewares(infraEnabled)
	if len(middlewaresEnabled) == 0 {
		t.Error("中间件链不应为空")
	}

	docDisabled := loadMiddlewareTestDoc(t, false)
	infraDisabled := &Infra{
		Doc:               docDisabled,
		HttpRecordService: httprecord.NewNopService(),
		AppCtx:            appctx.NewAppCtx("test", "test", "v1"),
	}
	middlewaresDisabled := DefaultMiddlewares(infraDisabled)
	if len(middlewaresEnabled) != len(middlewaresDisabled) {
		t.Errorf("auth.enabled 不应改变中间件链长度: enabled=%d disabled=%d", len(middlewaresEnabled), len(middlewaresDisabled))
	}
}
