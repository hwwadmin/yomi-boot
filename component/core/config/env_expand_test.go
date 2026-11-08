package config_test

import (
	"os"
	"strings"
	"testing"

	"github.com/hwwadmin/yomi-boot/component/core/config"
)

func TestExpandEnvDefaults_RejectsNewline(t *testing.T) {
	t.Setenv("APP_NAME", "demo\n  token: injected")
	got, err := config.ExpandEnvDefaults("app:\n  name: ${APP_NAME}\n")
	if err == nil {
		t.Fatal("替换值含换行时应失败")
	}
	if !strings.Contains(err.Error(), "APP_NAME") || !strings.Contains(err.Error(), "换行") {
		t.Fatalf("错误应点明占位符: %v", err)
	}
	if strings.Contains(got, "injected") || strings.Contains(err.Error(), "injected") {
		t.Fatalf("失败结果不应带上替换值: got=%q err=%v", got, err)
	}
}

func TestExpandEnvDefaults_RejectsCarriageReturn(t *testing.T) {
	t.Setenv("APP_NAME", "demo\r  token: injected")
	if _, err := config.ExpandEnvDefaults("name: ${APP_NAME}\n"); err == nil {
		t.Fatal("替换值含回车时应失败")
	}
}

func TestExpandEnvDefaults_RejectsNewlineInDefault(t *testing.T) {
	const name = "APP_NAME_NEWLINE_DEFAULT_TEST"
	orig, existed := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("取消环境变量: %v", err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(name, orig)
			return
		}
		_ = os.Unsetenv(name)
	})

	got, err := config.ExpandEnvDefaults("name: ${" + name + ":demo\n  token: injected}\n")
	if err == nil {
		t.Fatal("默认值含换行时应失败")
	}
	if !strings.Contains(err.Error(), name) {
		t.Fatalf("错误应点明占位符: %v", err)
	}
	if strings.Contains(got, "injected") || strings.Contains(err.Error(), "injected") {
		t.Fatalf("失败结果不应带上替换值: got=%q err=%v", got, err)
	}
}

func TestExpandEnvDefaults_UnsetUsesDefault(t *testing.T) {
	const name = "APP_NAME_UNSET_FOR_EXPAND_TEST"
	orig, existed := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("取消环境变量: %v", err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(name, orig)
			return
		}
		_ = os.Unsetenv(name)
	})

	got, err := config.ExpandEnvDefaults("name: ${" + name + ":demo}\n")
	if err != nil {
		t.Fatalf("未设置时应使用默认值: %v", err)
	}
	if got != "name: demo\n" {
		t.Fatalf("展开结果 = %q", got)
	}
}

func TestExpandEnvDefaults_EmptyOverridesDefault(t *testing.T) {
	t.Setenv("APP_NAME", "")
	got, err := config.ExpandEnvDefaults("name: ${APP_NAME:demo}\n")
	if err != nil {
		t.Fatalf("空字符串应允许展开: %v", err)
	}
	if got != "name: \n" {
		t.Fatalf("空字符串应盖过默认值: %q", got)
	}
}

func TestLoadConfig_EnvNewlineFails(t *testing.T) {
	t.Setenv("APP_NAME", "demo\n  token: injected")
	path := writeTempYAML(t, `
app:
  name: ${APP_NAME}
http:
  port: 8080
snowflake:
  worker_id: 1
`)
	cfg, err := config.LoadConfig(path, testRegistry())
	if err == nil || cfg != nil {
		t.Fatal("含换行的环境变量应让加载失败")
	}
	if !strings.Contains(err.Error(), "展开配置占位符失败") || !strings.Contains(err.Error(), "APP_NAME") {
		t.Fatalf("错误 = %v", err)
	}
	if strings.Contains(err.Error(), "injected") {
		t.Fatalf("错误不应带上替换值: %v", err)
	}
}
