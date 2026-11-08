package repo

import (
	"github.com/hwwadmin/yomi-boot/component/core/shared/enums"
	"strings"
	"testing"
)

// TestResetFetchedTimeoutSQL 验证两种数据库的超时回收语句都在库内限制条数
func TestResetFetchedTimeoutSQL(t *testing.T) {
	mysqlSQL, err := resetFetchedTimeoutSQL(string(enums.MySQL))
	if err != nil {
		t.Fatalf("MySQL 语句应可生成: %v", err)
	}
	if !strings.Contains(mysqlSQL, "ORDER BY fetch_time ASC") || !strings.Contains(mysqlSQL, "LIMIT ?") {
		t.Fatalf("MySQL 语句应按取走时间限制条数: %s", mysqlSQL)
	}
	if strings.Contains(mysqlSQL, "SELECT") {
		t.Fatalf("MySQL 语句应直接更新: %s", mysqlSQL)
	}

	postgresSQL, err := resetFetchedTimeoutSQL(string(enums.Postgres))
	if err != nil {
		t.Fatalf("PostgreSQL 语句应可生成: %v", err)
	}
	if !strings.Contains(postgresSQL, "SELECT id") || !strings.Contains(postgresSQL, "LIMIT ?") {
		t.Fatalf("PostgreSQL 语句应在子查询里限制条数: %s", postgresSQL)
	}

	if _, err := resetFetchedTimeoutSQL("sqlite"); err == nil {
		t.Fatal("未知数据库类型应返回错误")
	}
}
