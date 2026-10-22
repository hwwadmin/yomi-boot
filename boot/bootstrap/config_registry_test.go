// Package bootstrap 配置注册表集成测试
//
// @author zwei
package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/common/appctx"
	"gitee.com/hq_project/yomi-boot/core/config"
)

// TestLoad_RealServiceYAML 验证加载真实 user-service local.yml（可选）
func TestLoad_RealServiceYAML(t *testing.T) {
	path := filepath.Join("..", "..", "yomi-backend", "services", "user-service", "config", "local.yml")
	if _, err := os.Stat(path); err != nil {
		t.Skip("跳过：仓库相对路径下无 user-service local.yml")
	}
	doc, err := config.LoadDocument(path, "YOMI_USER", DefaultConfigRegistry())
	if err != nil {
		t.Fatalf("加载真实 YAML 失败: %v", err)
	}
	app := config.MustSection[appctx.Config](doc, appctx.SectionKey)
	if app.Name != "user-service" {
		t.Fatalf("app.name=%q", app.Name)
	}
}
