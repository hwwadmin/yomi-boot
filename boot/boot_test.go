package boot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/plugin"
	appctxplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/appctx"
	dbplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/db"
	eventplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/event"
	snowflakeplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/snowflake"
)

// TestMustInitPluginSlot 验证最小配置下默认插件链可初始化、可查找并可关闭
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

	slot := MustInit(context.Background(), Options{ConfigPath: configPath})
	closeSlot(t, slot)

	if got := appctxplugin.From(slot).AppCtx().Name(); got != "boot-test" {
		t.Fatalf("app.name = %q, want %q", got, "boot-test")
	}
	if !slot.Started(appctxplugin.Name) {
		t.Fatal("app 插件应处于已初始化状态")
	}
	if slot.Started(dbplugin.DBName) {
		t.Fatal("未启用 db 时 db 插件应为跳过状态")
	}
	if _, ok := slot.Plugin(eventplugin.Name); !ok {
		t.Fatal("event 插件应存在于插件链中")
	}
	if snowflakeplugin.From(slot).Generator() == nil {
		t.Fatal("snowflake 插件应提供 ID 生成器")
	}
}

// TestMustInitPluginSetUsesExactPlugins 验证 PluginSet 只使用给定的插件与配置段，不追加内置项
func TestMustInitPluginSetUsesExactPlugins(t *testing.T) {
	configPath := writeBootConfig(t, `app:
  name: plugin-set-test
  env: test
  timezone: UTC
`)

	slot := MustInit(context.Background(), Options{
		ConfigPath: configPath,
		PluginSet: &PluginSet{
			Plugins: []plugin.Plugin{appctxplugin.NewAppCtx()},
			Configs: []config.Section{appctxplugin.ConfigSection()},
		},
	})
	closeSlot(t, slot)

	if got := len(slot.Plugins()); got != 1 {
		t.Fatalf("插件数量 = %d, want 1", got)
	}
	if got := appctxplugin.From(slot).AppCtx().Name(); got != "plugin-set-test" {
		t.Fatalf("app.name = %q, want %q", got, "plugin-set-test")
	}
	if _, ok := slot.Plugin(dbplugin.DBName); ok {
		t.Fatal("PluginSet 模式不应包含内置 db 插件")
	}
	if _, ok := slot.Plugin(eventplugin.Name); ok {
		t.Fatal("PluginSet 模式不应包含内置 event 插件")
	}
}

// TestMustInitPluginSetWithCustomPlugin 验证自定义插件与自定义配置段可直接装配
func TestMustInitPluginSetWithCustomPlugin(t *testing.T) {
	type bizConfig struct {
		Flag bool `mapstructure:"flag"`
	}

	biz := &bizConfig{}
	custom := newTestPlugin("test-plugin")
	configPath := writeBootConfig(t, "biz:\n  flag: true\n")

	slot := MustInit(context.Background(), Options{
		ConfigPath: configPath,
		PluginSet: &PluginSet{
			Plugins: []plugin.Plugin{custom},
			Configs: []config.Section{config.NewSection("biz", biz, nil)},
		},
	})
	closeSlot(t, slot)

	if !slot.Started("test-plugin") {
		t.Fatal("自定义插件应已初始化")
	}
	if !biz.Flag {
		t.Fatal("自定义配置段应已加载")
	}
	if _, ok := slot.Plugin(dbplugin.DBName); ok {
		t.Fatal("PluginSet 模式不应包含内置 db 插件")
	}
}

// TestMustInitPluginSetIgnoresCustomPlugins 验证 PluginSet 模式下忽略 PluginOptions 自定义插件
func TestMustInitPluginSetIgnoresCustomPlugins(t *testing.T) {
	configPath := writeBootConfig(t, `app:
  name: plugin-set-test
  env: test
  timezone: UTC
`)
	opts := plugin.PluginOptions{}
	opts.AddCustomPlugin(newTestPlugin("ignored-custom"))

	slot := MustInit(context.Background(), Options{
		ConfigPath: configPath,
		Plugin:     opts,
		PluginSet: &PluginSet{
			Plugins: []plugin.Plugin{appctxplugin.NewAppCtx()},
			Configs: []config.Section{appctxplugin.ConfigSection()},
		},
	})
	closeSlot(t, slot)

	if slot.Started("ignored-custom") {
		t.Fatal("PluginSet 模式应忽略 PluginOptions 自定义插件")
	}
	if got := len(slot.Plugins()); got != 1 {
		t.Fatalf("插件数量 = %d, want 1", got)
	}
}

// TestMustInitPluginSetIgnoresCustomConfigs 验证 PluginSet 模式下忽略 PluginOptions 自定义配置段
func TestMustInitPluginSetIgnoresCustomConfigs(t *testing.T) {
	type bizConfig struct {
		Flag bool `mapstructure:"flag"`
	}

	opts := plugin.PluginOptions{}
	opts.AddConfigSection(config.NewSection("biz", &bizConfig{}, nil))
	configPath := writeBootConfig(t, `app:
  name: plugin-set-test
  env: test
  timezone: UTC
biz:
  flag: true
`)

	expectPanic(t, "未注册段", func() {
		MustInit(context.Background(), Options{
			ConfigPath: configPath,
			Plugin:     opts,
			PluginSet: &PluginSet{
				Plugins: []plugin.Plugin{appctxplugin.NewAppCtx()},
				Configs: []config.Section{appctxplugin.ConfigSection()},
			},
		})
	})
}

// TestMustInitPluginSetDoesNotRegisterDefaultConfigs 验证 PluginSet 模式不注册内置默认配置段
func TestMustInitPluginSetDoesNotRegisterDefaultConfigs(t *testing.T) {
	configPath := writeBootConfig(t, `app:
  name: plugin-set-test
  env: test
  timezone: UTC
db:
  enabled: false
`)

	expectPanic(t, "未注册段", func() {
		MustInit(context.Background(), Options{
			ConfigPath: configPath,
			PluginSet: &PluginSet{
				Plugins: []plugin.Plugin{appctxplugin.NewAppCtx()},
				Configs: []config.Section{appctxplugin.ConfigSection()},
			},
		})
	})
}

// TestMustInitPluginSetAllowsEmptySet 验证空 PluginSet 表示不装配任何插件与配置段
func TestMustInitPluginSetAllowsEmptySet(t *testing.T) {
	slot := MustInit(context.Background(), Options{
		ConfigPath: writeBootConfig(t, "{}\n"),
		PluginSet:  &PluginSet{},
	})
	closeSlot(t, slot)

	if got := len(slot.Plugins()); got != 0 {
		t.Fatalf("插件数量 = %d, want 0", got)
	}
}

// testPlugin 是仅用于 PluginSet 测试的最小插件
type testPlugin struct {
	plugin.Base
}

func newTestPlugin(name string) *testPlugin {
	return &testPlugin{Base: plugin.NewBase(name)}
}

func (p *testPlugin) Init(context.Context, *plugin.PluginSlot) error {
	return nil
}

func (p *testPlugin) Close(context.Context) error {
	return nil
}

// closeSlot 在测试结束时关闭插件插槽
func closeSlot(t *testing.T, slot *plugin.PluginSlot) {
	t.Helper()
	t.Cleanup(func() {
		if err := slot.Close(context.Background()); err != nil {
			t.Errorf("关闭插件插槽失败: %v", err)
		}
	})
}

// writeBootConfig 写入临时 YAML 配置并返回路径
func writeBootConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "app.yml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}
	return path
}

// expectPanic 断言 fn 触发 panic 且信息包含 want
func expectPanic(t *testing.T, want string, fn func()) {
	t.Helper()

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("期望 panic，实际正常返回")
		}
		if msg := fmt.Sprint(recovered); !strings.Contains(msg, want) {
			t.Fatalf("panic 信息 = %q, want contains %q", msg, want)
		}
	}()

	fn()
}
