// DTM 配置的单元测试
//
// @author zwei

package dtm

import "testing"

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
