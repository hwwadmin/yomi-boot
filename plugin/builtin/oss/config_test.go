// 对象存储 YAML 绑定单测。

package ossplugin

import (
	"testing"

	"gitee.com/hq_project/yomi-boot/component/common/oss"
)

// TestBootConfigToConfig 验证 BootConfig 映射运行时 Config 的 provider 与 PublicBaseURL
func TestBootConfigToConfig(t *testing.T) {
	t.Parallel()

	cfg := BootConfig{
		Provider:      " RustFS ",
		PublicBaseURL: " https://cdn.example.com/ ",
		RustFS: BootRustFSConfig{
			Endpoint:        "127.0.0.1:9000",
			AccessKeyID:     "ak",
			SecretAccessKey: "sk",
		},
	}.ToConfig()

	if cfg.Provider != oss.ProviderRustFS {
		t.Fatalf("Provider: got %q, want %q", cfg.Provider, oss.ProviderRustFS)
	}
	if cfg.PublicBaseURL != "https://cdn.example.com/" {
		t.Fatalf("PublicBaseURL: got %q", cfg.PublicBaseURL)
	}
}

// TestBootConfigValidate 验证未启用跳过校验、启用后按 provider 校验必填项
func TestBootConfigValidate(t *testing.T) {
	t.Parallel()

	if err := (BootConfig{}).Validate(); err != nil {
		t.Fatalf("未配置 enabled 时应跳过校验: %v", err)
	}

	enabled := true
	t.Run("provider 为空", func(t *testing.T) {
		err := BootConfig{Enabled: &enabled}.Validate()
		if err == nil {
			t.Fatal("provider 为空应报错")
		}
	})
	t.Run("rustfs 缺 endpoint", func(t *testing.T) {
		err := BootConfig{Enabled: &enabled, Provider: "rustfs"}.Validate()
		if err == nil {
			t.Fatal("rustfs.endpoint 为空应报错")
		}
	})
}
