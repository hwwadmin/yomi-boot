package appctxplugin

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/appctx"
	"gitee.com/hq_project/yomi-boot/config"
	"gitee.com/hq_project/yomi-boot/plugin"
)

// TestAppCtxPlugin_LocksTimezone 验证 app 插件启动时按 app.timezone 锁定进程时区
func TestAppCtxPlugin_LocksTimezone(t *testing.T) {
	original := time.Local
	t.Cleanup(func() { time.Local = original })

	cases := []struct {
		name string
		line string
		want string
	}{
		{name: "configured", line: "  timezone: Asia/Shanghai\n", want: "Asia/Shanghai"},
		{name: "default", line: "", want: appctx.DefaultTimezone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yml")
			content := "app:\n  name: test\n" + tc.line
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatalf("写临时配置失败: %v", err)
			}
			reg := config.NewRegistry().Register(appctx.ConfigSection())
			doc, err := config.LoadDocument(path, "YOMI_APPCONTEXT_TEST", reg)
			if err != nil {
				t.Fatalf("加载临时配置失败: %v", err)
			}

			host := &plugin.Host{Doc: doc}
			NewAppCtx().Init(host)

			if host.AppCtx == nil || host.AppCtx.Name() != "test" {
				t.Fatal("AppCtx 未正确初始化")
			}
			if got := time.Local.String(); got != tc.want {
				t.Fatalf("time.Local = %s, want %s", got, tc.want)
			}
		})
	}
}
