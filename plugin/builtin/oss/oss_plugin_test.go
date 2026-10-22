// 对象存储插件单测。

package ossplugin

import (
	"os"
	"path/filepath"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/plugin"
)

// loadOSSDoc 写入并加载 oss 单测配置文档
func loadOSSDoc(t *testing.T, yaml string) *config.Document {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("写临时配置失败: %v", err)
	}
	doc, err := config.LoadDocument(path, "YOMI_OSS_TEST", config.NewRegistry().Register(ConfigSection()))
	if err != nil {
		t.Fatalf("加载临时配置失败: %v", err)
	}
	return doc
}

// TestOSSPluginDisabled 验证未启用时不创建 Storage、不标记 Started
func TestOSSPluginDisabled(t *testing.T) {
	t.Parallel()

	host := &plugin.Host{Services: plugin.Services{Doc: loadOSSDoc(t, "oss: {}")}}
	p := NewOSS().(*ossPlugin)
	p.Init(host)
	if p.Started() {
		t.Fatal("oss 未启用时不应标记 Started")
	}
	if host.OSS != nil {
		t.Fatal("oss 未启用时不应创建 Storage")
	}
}

// TestOSSPluginEnabled 验证启用后构造 Storage 并写入 Host
func TestOSSPluginEnabled(t *testing.T) {
	t.Parallel()

	doc := loadOSSDoc(t, `
oss:
  enabled: true
  provider: rustfs
  public_base_url: https://cdn.example.com
  rustfs:
    endpoint: 127.0.0.1:9000
    access_key_id: ak
    secret_access_key: sk
`)
	host := &plugin.Host{Services: plugin.Services{Doc: doc}}
	p := NewOSS().(*ossPlugin)
	p.Init(host)
	if !p.Started() {
		t.Fatal("oss 启用后应标记 Started")
	}
	if host.OSS == nil {
		t.Fatal("oss 启用后应写入 Host.OSS")
	}
	url, err := host.OSS.ObjectURL("avatars/u/1.png")
	if err != nil {
		t.Fatalf("ObjectURL 失败: %v", err)
	}
	if url != "https://cdn.example.com/avatars/u/1.png" {
		t.Fatalf("ObjectURL = %q", url)
	}
}
