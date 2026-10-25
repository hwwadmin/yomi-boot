// DTM 配置的单元测试
//
// @author zwei

package dtm

import (
	"testing"
	"time"
)

// TestTimeoutOrDefault 验证控制面超时的默认值与非正数回退
func TestTimeoutOrDefault(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   int
		want time.Duration
	}{
		{name: "未配置使用默认值", in: 0, want: DefaultTimeout},
		{name: "负数使用默认值", in: -3, want: DefaultTimeout},
		{name: "正数按秒生效", in: 7, want: 7 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := (Config{Timeout: tc.in}).TimeoutOrDefault(); got != tc.want {
				t.Fatalf("TimeoutOrDefault(%d)=%s, want %s", tc.in, got, tc.want)
			}
		})
	}
}

// TestConfigValidateTimeout 验证负数超时被拒绝（0 合法，由 TimeoutOrDefault 回退默认值）
func TestConfigValidateTimeout(t *testing.T) {
	t.Parallel()

	if err := (Config{Timeout: -1}).Validate(); err == nil {
		t.Fatal("负数 timeout 期望报错")
	}
	if err := (Config{Timeout: 0}).Validate(); err != nil {
		t.Fatalf("0 timeout 应合法，实际: %v", err)
	}
}

// TestConfigValidateServer 验证 server 为空放行、非法 server 报错
func TestConfigValidateServer(t *testing.T) {
	t.Parallel()

	if err := (Config{}).Validate(); err != nil {
		t.Fatalf("空配置应合法，实际: %v", err)
	}
	if err := (Config{Server: "   "}).Validate(); err != nil {
		t.Fatalf("空白 server 应合法，实际: %v", err)
	}
	if err := (Config{Server: "http://"}).Validate(); err == nil {
		t.Fatal("缺少 host 的 server 期望报错")
	}
}

// TestResolveServer 验证 server 地址的协议与路径规范化
func TestResolveServer(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "空地址", in: "", want: ""},
		{name: "补全协议与路径", in: "127.0.0.1:36789", want: "http://127.0.0.1:36789/api/dtmsvr"},
		{name: "已带协议仅补路径", in: "http://127.0.0.1:36789", want: "http://127.0.0.1:36789/api/dtmsvr"},
		{name: "保留已配置路径", in: "http://127.0.0.1:36789/dtm", want: "http://127.0.0.1:36789/dtm"},
		{name: "根路径补全", in: "http://127.0.0.1:36789/", want: "http://127.0.0.1:36789/api/dtmsvr"},
		{name: "解析失败回退拼接", in: "bad\x7f", want: "http://bad\x7f"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := (Config{Server: tc.in}).ResolveServer(); got != tc.want {
				t.Fatalf("ResolveServer(%q)=%q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestBarrierTableOrDefault 验证 Barrier 表名默认值与首尾空白裁剪
func TestBarrierTableOrDefault(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "未配置使用默认值", in: "", want: DefaultBarrierTableName},
		{name: "空白使用默认值", in: "   ", want: DefaultBarrierTableName},
		{name: "显式配置", in: "custom_barrier", want: "custom_barrier"},
		{name: "去除首尾空白", in: " custom_barrier ", want: "custom_barrier"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := (Config{BarrierTable: tc.in}).BarrierTableOrDefault(); got != tc.want {
				t.Fatalf("BarrierTableOrDefault()=%q, want %q", got, tc.want)
			}
		})
	}
}

// TestConfigValidateBarrierTable 验证非法 Barrier 表名被拒绝
func TestConfigValidateBarrierTable(t *testing.T) {
	t.Parallel()

	base := Config{Server: "http://127.0.0.1:36789", Timeout: 3}
	cases := []struct {
		name    string
		table   string
		wantErr bool
	}{
		{name: "默认合法", table: "", wantErr: false},
		{name: "合法标识符", table: "custom_barrier", wantErr: false},
		{name: "数字开头非法", table: "1barrier", wantErr: true},
		{name: "含分号非法", table: "sys_barrier; DROP TABLE x", wantErr: true},
		{name: "含空格非法", table: "sys barrier", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := base
			cfg.BarrierTable = tc.table
			err := cfg.Validate()
			if tc.wantErr && err == nil {
				t.Fatalf("BarrierTable=%q 期望报错", tc.table)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("BarrierTable=%q 期望合法，实际 %v", tc.table, err)
			}
		})
	}
}
