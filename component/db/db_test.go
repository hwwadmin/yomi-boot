// Database DSN 的单元测试
//
// @author zwei

package db

import (
	"strings"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/boot/shared/enums"

	"github.com/go-sql-driver/mysql"
)

// TestWithMySQLDefaults 验证 MySQL 参数默认值补齐与显式配置保留
func TestWithMySQLDefaults(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		params string
		want   string
	}{
		{name: "空参数补默认值", params: "", want: "parseTime=true&loc=Local"},
		{name: "保留已有参数并补默认值", params: "charset=utf8mb4", want: "charset=utf8mb4&parseTime=true&loc=Local"},
		{name: "显式parseTime不被覆盖", params: "parseTime=false", want: "parseTime=false&loc=Local"},
		{name: "显式loc不被覆盖", params: "loc=UTC", want: "loc=UTC&parseTime=true"},
		{name: "两者均显式保持原样", params: "parseTime=true&loc=Asia%2FShanghai", want: "parseTime=true&loc=Asia%2FShanghai"},
		{name: "去掉误带的前导问号", params: "?charset=utf8mb4", want: "charset=utf8mb4&parseTime=true&loc=Local"},
		{name: "忽略空片段", params: "charset=utf8mb4&&", want: "charset=utf8mb4&parseTime=true&loc=Local"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := withMySQLDefaults(tc.params); got != tc.want {
				t.Fatalf("withMySQLDefaults(%q)=%q, want %q", tc.params, got, tc.want)
			}
		})
	}
}

// TestBuildDSN_MySQLDefaultsRecognized 验证 MySQL 默认参数可被驱动识别，且 Postgres DSN 不受影响
func TestBuildDSN_MySQLDefaultsRecognized(t *testing.T) {
	t.Parallel()

	dsn, err := buildDSN(Config{
		Type: enums.MySQL, Host: "127.0.0.1", Port: 3306,
		User: "u", Password: "p", Database: "d",
	})
	if err != nil {
		t.Fatalf("buildDSN 失败: %v", err)
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("驱动无法解析 MySQL DSN: %v", err)
	}
	if !cfg.ParseTime {
		t.Fatal("期望 parseTime 默认为 true")
	}
	if cfg.Loc != time.Local {
		t.Fatalf("期望 loc 默认为 Local，实际 %v", cfg.Loc)
	}

	pgDSN, err := buildDSN(Config{
		Type: enums.Postgres, Host: "127.0.0.1", Port: 5432,
		User: "u", Password: "p", Database: "d",
	})
	if err != nil {
		t.Fatalf("buildDSN 失败: %v", err)
	}
	if strings.Contains(pgDSN, "parseTime") || strings.Contains(pgDSN, "loc=") {
		t.Fatalf("Postgres DSN 不应包含 MySQL 参数: %q", pgDSN)
	}
}
