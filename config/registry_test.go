// Package config_test 配置注册与加载单测（外部测试包，避免与能力包 import cycle）
//
// @author zwei
package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"yt-boot/appctx"
	"yt-boot/bootstrap"
	"yt-boot/config"
	"yt-boot/router"
	"yt-boot/snowflake"
)

// testInfraRegistry 返回配置加载单测用注册表
func testInfraRegistry() *config.Registry {
	return config.NewRegistry().Register(
		appctx.ConfigSection(),
		router.ConfigSection(),
		snowflake.ConfigSection(),
	)
}

// TestLoadDocument_StrictUnknownRootKey 验证未注册根 key 时加载失败
func TestLoadDocument_StrictUnknownRootKey(t *testing.T) {
	path := writeTempYAML(t, `
app:
  name: demo
http:
  port: 8080
snowflake:
  worker_id: 1
unknown_biz:
  foo: 1
`)
	_, err := config.LoadDocument(path, "YT_TEST", testInfraRegistry())
	if err == nil {
		t.Fatal("期望未注册根 key 失败")
	}
	if !strings.Contains(err.Error(), "配置存在未注册段") || !strings.Contains(err.Error(), "unknown_biz") {
		t.Fatalf("错误信息不符合严格策略: %v", err)
	}
}

// TestLoadDocument_OK 验证合法 YAML 加载与 MustSection 取值
func TestLoadDocument_OK(t *testing.T) {
	path := writeTempYAML(t, `
app:
  name: demo
http:
  port: 8080
  router_prefix: /admin/api
snowflake:
  worker_id: 1
`)
	doc, err := config.LoadDocument(path, "YT_TEST", testInfraRegistry())
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	app := config.MustSection[appctx.Config](doc, appctx.SectionKey)
	if app.Name != "demo" {
		t.Fatalf("app.name=%q", app.Name)
	}
	httpCfg := config.MustSection[router.Config](doc, router.SectionKey)
	if httpCfg.Port != 8080 {
		t.Fatalf("http.port=%d", httpCfg.Port)
	}
}

// TestLoadDocument_CustomBizSection 验证自定义业务配置段注册与加载
func TestLoadDocument_CustomBizSection(t *testing.T) {
	path := writeTempYAML(t, `
app:
  name: demo
http:
  port: 8080
snowflake:
  worker_id: 1
biz:
  flag: true
`)
	type bizConfig struct {
		Flag bool `mapstructure:"flag"`
	}
	const bizSectionKey = "biz"
	biz := &bizConfig{}
	reg := testInfraRegistry().Register(config.NewSection(bizSectionKey, biz, nil))
	doc, err := config.LoadDocument(path, "YT_TEST", reg)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	got := config.MustSection[bizConfig](doc, bizSectionKey)
	if !got.Flag {
		t.Fatal("期望 biz.flag=true")
	}
}

// TestRegistry_DuplicateKeyPanics 验证重复 key 注册 panic
func TestRegistry_DuplicateKeyPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("期望重复 key panic")
		}
	}()
	testInfraRegistry().Register(config.NewSection(appctx.SectionKey, &appctx.Config{}, nil))
}

// TestLoad_RealServiceYAML 验证加载真实 user-service local.yml（可选）
func TestLoad_RealServiceYAML(t *testing.T) {
	path := filepath.Join("..", "yt-backend", "services", "user-service", "config", "local.yml")
	if _, err := os.Stat(path); err != nil {
		t.Skip("跳过：仓库相对路径下无 user-service local.yml")
	}
	doc, err := config.LoadDocument(path, "YT_USER", bootstrap.DefaultConfigRegistry())
	if err != nil {
		t.Fatalf("加载真实 YAML 失败: %v", err)
	}
	app := config.MustSection[appctx.Config](doc, appctx.SectionKey)
	if app.Name != "user-service" {
		t.Fatalf("app.name=%q", app.Name)
	}
}

// writeTempYAML 写入临时 YAML 配置文件并返回路径
func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写临时配置失败: %v", err)
	}
	return path
}
