package consulplugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/common/appctx"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/plugin"
)

// pathRecorder 记录 Consul 请求路径
type pathRecorder struct {
	mu    sync.Mutex
	paths []string
}

// add 记录一次请求路径
func (r *pathRecorder) add(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paths = append(r.paths, path)
}

// snapshot 返回当前路径快照
func (r *pathRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.paths))
	copy(out, r.paths)
	return out
}

// TestConsulPluginRegisterOnReadyAndDeregisterOnDraining 验证注册/注销只在 Ready/Drain 阶段发生
func TestConsulPluginRegisterOnReadyAndDeregisterOnDraining(t *testing.T) {
	t.Parallel()

	recorder := &pathRecorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.add(r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	doc := loadConsulDoc(t, `
consul:
  enabled: true
  addr: `+server.URL+`
  service_id: svc-1
  service_address: 127.0.0.1:8080
  check_address: http://127.0.0.1:8080/health
`)
	host := &plugin.Host{
		Doc:    doc,
		AppCtx: appctx.NewAppCtx("test", "dev", "v1"),
	}
	p := NewConsul().(*consulPlugin)
	p.Init(host)
	if !p.Started() {
		t.Fatal("consul 启用后插件应标记 Started")
	}
	if got := recorder.snapshot(); len(got) != 0 {
		t.Fatalf("Init 阶段不应发起注册请求: %v", got)
	}

	if err := p.OnReady(context.Background()); err != nil {
		t.Fatalf("OnReady 失败: %v", err)
	}
	got := recorder.snapshot()
	if len(got) != 1 || got[0] != "/v1/agent/service/register" {
		t.Fatalf("Ready 阶段请求 = %v, want [/v1/agent/service/register]", got)
	}

	if err := p.OnDraining(context.Background()); err != nil {
		t.Fatalf("OnDraining 失败: %v", err)
	}
	got = recorder.snapshot()
	if len(got) != 2 || got[1] != "/v1/agent/service/deregister/svc-1" {
		t.Fatalf("Drain 阶段请求 = %v, want 第二步注销 svc-1", got)
	}
}

// TestConsulPluginDisabled 验证未启用时不创建客户端、不标记 Started
func TestConsulPluginDisabled(t *testing.T) {
	t.Parallel()

	doc := loadConsulDoc(t, `
consul:
  enabled: false
`)
	p := NewConsul().(*consulPlugin)
	p.Init(&plugin.Host{Doc: doc})
	if p.Started() {
		t.Fatal("consul 未启用时不应标记 Started")
	}
	if p.client != nil {
		t.Fatal("consul 未启用时不应创建客户端")
	}
}

// loadConsulDoc 写入并加载 consul 单测配置
func loadConsulDoc(t *testing.T, content string) *config.Document {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写临时配置失败: %v", err)
	}
	reg := config.NewRegistry().Register(ConfigSection())
	doc, err := config.LoadDocument(path, "YOMI_CONSUL_PLUGIN_TEST", reg)
	if err != nil {
		t.Fatalf("加载临时配置失败: %v", err)
	}
	return doc
}
