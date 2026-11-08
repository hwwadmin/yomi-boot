package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hwwadmin/yomi-boot/component/core/config"
)

func TestBoolOrAndResolvePath(t *testing.T) {
	if !config.BoolOr(nil, true) {
		t.Fatal("nil 应返回默认值")
	}
	off := false
	if config.BoolOr(&off, true) {
		t.Fatal("显式 false 不应被默认值覆盖")
	}
	if config.BoolOrDefault(nil) {
		t.Fatal("未配置应默认 false")
	}
	on := true
	if !config.BoolOrDefault(&on) {
		t.Fatal("显式 true 应保留")
	}

	t.Setenv("YOMI_CONFIG", " local ")
	if got := config.ResolvePathFromDir("cfg"); got != filepath.Join("cfg", "local.yml") {
		t.Fatalf("ResolvePathFromDir = %q", got)
	}
	t.Setenv("YOMI_CONFIG", "nope")
	if got := config.ResolvePathFromDir("cfg"); !strings.HasSuffix(got, filepath.Join("cfg", "dev.yml")) && got != filepath.Join("cfg", "dev.yml") {
		t.Fatalf("非法环境应回退 dev，实际 %q", got)
	}
	if config.ResolvePath() == "" {
		t.Fatal("ResolvePath 不应为空")
	}
}

func expectPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("应 panic")
		}
	}()
	fn()
}

func TestRegistryNilAndEmptyKey(t *testing.T) {
	var reg *config.Registry
	if reg.Has("app") || reg.Keys() != nil {
		t.Fatal("nil 注册表应为空")
	}
	expectPanic(t, func() { reg.Register() })

	reg = &config.Registry{}
	target := &testAppConfig{}
	reg.Register(config.NewSection("app", target, nil))
	if !reg.Has(" app ") || len(reg.Keys()) != 1 {
		t.Fatalf("keys=%v has=%v", reg.Keys(), reg.Has("app"))
	}
	expectPanic(t, func() { reg.Register(nil) })
	expectPanic(t, func() { config.NewSection[testAppConfig]("empty", nil, nil) })
	expectPanic(t, func() {
		reg.Register(config.NewSection("   ", &testAppConfig{}, nil))
	})
}

func TestConfigAccessorsAndMustSection(t *testing.T) {
	var cfg *config.Config
	if cfg.Has("app") || cfg.Keys() != nil {
		t.Fatal("nil 配置应为空")
	}
	expectPanic(t, func() { config.MustSection[testAppConfig](cfg, "app") })

	path := writeTempYAML(t, "app:\n  name: demo\n")
	loaded, err := config.LoadConfig(path, config.NewRegistry().Register(
		config.NewSection(testAppSectionKey, &testAppConfig{}, nil),
	))
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if !loaded.Has(" app ") || len(loaded.Keys()) != 1 {
		t.Fatalf("keys=%v", loaded.Keys())
	}
	expectPanic(t, func() { config.MustSection[testAppConfig](loaded, "missing") })
	expectPanic(t, func() { config.MustSection[testHTTPConfig](loaded, testAppSectionKey) })
}

func TestLoadConfig_FailureEdges(t *testing.T) {
	if _, err := config.LoadConfig("missing.yml", nil); err == nil {
		t.Fatal("空注册表应失败")
	}
	missing := filepath.Join(t.TempDir(), "missing.yml")
	if _, err := config.LoadConfig(missing, config.NewRegistry()); err == nil {
		t.Fatal("缺少文件应失败")
	}
	bad := writeTempYAML(t, ":\n  - [")
	if _, err := config.LoadConfig(bad, config.NewRegistry()); err == nil {
		t.Fatal("非法 YAML 应失败")
	}
	path := writeTempYAML(t, "app:\n  name: demo\n")
	reg := config.NewRegistry().Register(config.NewSection(testAppSectionKey, &testAppConfig{}, func(*testAppConfig, *config.Config) error {
		return errors.New("bad section")
	}))
	if _, err := config.LoadConfig(path, reg); err == nil {
		t.Fatal("校验失败应返回错误")
	}

	expectPanic(t, func() { config.MustLoadConfig(missing, config.NewRegistry()) })
}

func TestExpandEnvDefaults_StopsAfterNewline(t *testing.T) {
	t.Setenv("YOMI_NEWLINE", "a\nb")
	if _, err := config.ExpandEnvDefaults("${YOMI_NEWLINE} ${YOMI_OTHER:ok}"); err == nil {
		t.Fatal("替换值含换行应失败")
	}
	_ = os.Unsetenv("YOMI_NEWLINE")
}
