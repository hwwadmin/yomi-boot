package boot

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gitee.com/hq_project/yomi-boot/plugin"
	appctxplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/appctx"
	dbplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/db"
	eventplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/event"
	snowflakeplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/snowflake"
)

// TestMustInitPluginSlot 验证最小配置下插件链可初始化、可查找并可关闭
func TestMustInitPluginSlot(t *testing.T) {
	dir := t.TempDir()
	logDir := filepath.ToSlash(filepath.Join(dir, "logs"))
	configPath := filepath.Join(dir, "app.yml")
	content := fmt.Sprintf(`app:
  name: boot-test
  env: test
  timezone: UTC
log:
  dir: %s
  rotation:
    max_size_mb: 1
    retention_days: 1
    max_backups: 1
    max_total_mb: 1
http:
  port: 18080
snowflake:
  worker_id: 1
`, logDir)
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}

	slot := MustInit(configPath, "", plugin.PluginOptions{})
	t.Cleanup(slot.Close)

	if got := appctxplugin.From(slot).AppCtx().Name(); got != "boot-test" {
		t.Fatalf("app.name = %q, want %q", got, "boot-test")
	}
	if !slot.Started(appctxplugin.Name) {
		t.Fatal("app 插件应处于已启动状态")
	}
	if slot.Started(dbplugin.DBName) {
		t.Fatal("未启用 db 时不应启动 db 插件")
	}
	if _, ok := slot.Plugin(eventplugin.Name); !ok {
		t.Fatal("event 插件应存在于插件链中")
	}
	if snowflakeplugin.From(slot).Generator() == nil {
		t.Fatal("snowflake 插件应提供 ID 生成器")
	}
}
