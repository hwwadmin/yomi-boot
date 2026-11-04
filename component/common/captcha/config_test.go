// 配置校验测试。
//
// @author zwei

package captcha

import (
	"strings"
	"testing"
	"time"

	"github.com/hwwadmin/yomi-boot/component/core/errors"
)

// TestConfigNormalizeDefaults 验证零值配置填充默认值
func TestConfigNormalizeDefaults(t *testing.T) {
	normalized, err := (Config{}).normalize()
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if normalized.Driver != DriverDigit {
		t.Fatalf("driver = %q, 期望 digit", normalized.Driver)
	}
	if normalized.Length != defaultLength {
		t.Fatalf("length = %d, 期望 %d", normalized.Length, defaultLength)
	}
	if normalized.Width != defaultWidth {
		t.Fatalf("width = %d, 期望 %d", normalized.Width, defaultWidth)
	}
	if normalized.Height != defaultHeight {
		t.Fatalf("height = %d, 期望 %d", normalized.Height, defaultHeight)
	}
	if normalized.TTL != defaultTTL {
		t.Fatalf("ttl = %s, 期望 %s", normalized.TTL, defaultTTL)
	}
}

// TestConfigNormalizeStringSource 验证 string 驱动空 source 使用内置字符集
func TestConfigNormalizeStringSource(t *testing.T) {
	normalized, err := (Config{Driver: DriverString}).normalize()
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if normalized.Source != defaultSource {
		t.Fatalf("source = %q, 期望 %q", normalized.Source, defaultSource)
	}
}

// TestConfigValidate 验证配置校验规则
func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{name: "默认配置", cfg: Config{}},
		{name: "driver 非法", cfg: Config{Driver: "unknown"}, wantErr: true},
		{name: "digit length 过小", cfg: Config{Length: minLength - 1}, wantErr: true},
		{name: "digit length 过大", cfg: Config{Length: maxLength + 1}, wantErr: true},
		{name: "digit length 边界", cfg: Config{Length: maxLength}},
		{name: "math 忽略 length", cfg: Config{Driver: DriverMath, Length: -1}},
		{name: "width 过小", cfg: Config{Width: minWidth - 1}, wantErr: true},
		{name: "width 过大", cfg: Config{Width: maxWidth + 1}, wantErr: true},
		{name: "height 过小", cfg: Config{Height: minHeight - 1}, wantErr: true},
		{name: "height 过大", cfg: Config{Height: maxHeight + 1}, wantErr: true},
		{name: "ttl 过小", cfg: Config{TTL: minTTL - time.Second}, wantErr: true},
		{name: "ttl 过大", cfg: Config{TTL: maxTTL + time.Second}, wantErr: true},
		{name: "ttl 边界", cfg: Config{TTL: maxTTL}},
		{name: "string source 过短", cfg: Config{Driver: DriverString, Source: "ab"}, wantErr: true},
		{name: "string source 含非法字符", cfg: Config{Driver: DriverString, Source: "abcd-efg"}, wantErr: true},
		{name: "string source 含中文", cfg: Config{Driver: DriverString, Source: "测试字符集测试"}, wantErr: true},
		{name: "string source 合法", cfg: Config{Driver: DriverString, Source: "ABCDEFGH"}},
		{name: "string noise 过大", cfg: Config{Driver: DriverString, NoiseCount: maxNoiseCount + 1}, wantErr: true},
		{name: "math noise 过大", cfg: Config{Driver: DriverMath, NoiseCount: maxNoiseCount + 1}, wantErr: true},
		{name: "digit 忽略 noise", cfg: Config{Driver: DriverDigit, NoiseCount: maxNoiseCount + 1}},
		{name: "string lines 未知位", cfg: Config{Driver: DriverString, Lines: LineSine << 1}, wantErr: true},
		{name: "string lines 合法组合", cfg: Config{Driver: DriverString, Lines: LineHollow | LineSine}},
		{name: "digit 忽略 lines", cfg: Config{Driver: DriverDigit, Lines: LineSine << 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr && err == nil {
				t.Fatal("期望返回错误，实际为 nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("不期望错误，实际为 %v", err)
			}
			if tt.wantErr && !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("配置错误应携带 ErrInvalidArgument: %v", err)
			}
		})
	}
}

// TestValidateSource 验证字符源长度与字符集边界
func TestValidateSource(t *testing.T) {
	if err := validateSource(strings.Repeat("A", minSourceLen)); err != nil {
		t.Fatalf("最短 source 应合法: %v", err)
	}
	if err := validateSource(strings.Repeat("A", minSourceLen-1)); err == nil {
		t.Fatal("过短 source 应失败")
	}
	if err := validateSource(strings.Repeat("a", maxSourceLen)); err != nil {
		t.Fatalf("最长 source 应合法: %v", err)
	}
	if err := validateSource(strings.Repeat("a", maxSourceLen+1)); err == nil {
		t.Fatal("过长 source 应失败")
	}
	if err := validateSource("0123456789"); err != nil {
		t.Fatalf("纯数字 source 应合法: %v", err)
	}
}
