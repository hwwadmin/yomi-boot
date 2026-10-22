// GORM 日志适配的单元测试
//
// @author zwei

package db

import (
	"context"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/core/errors"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// TestGormLog_ParamsFilter 验证日志 SQL 不携带绑定参数
func TestGormLog_ParamsFilter(t *testing.T) {
	t.Parallel()

	l := newGormLogger(Config{}).(*gormLog)
	sql, params := l.ParamsFilter(context.Background(), "SELECT * FROM t WHERE id = ?", 42)
	if sql != "SELECT * FROM t WHERE id = ?" {
		t.Fatalf("SQL 不应被改写，实际 %q", sql)
	}
	if params != nil {
		t.Fatalf("期望丢弃绑定参数，实际 %v", params)
	}
}

// TestGormLog_LogMode 验证 LogMode 返回副本且不改原实例
func TestGormLog_LogMode(t *testing.T) {
	t.Parallel()

	l := &gormLog{level: gormlogger.Warn, slowThreshold: time.Second}
	got, ok := l.LogMode(gormlogger.Info).(*gormLog)
	if !ok {
		t.Fatal("LogMode 应返回 *gormLog")
	}
	if got == l {
		t.Fatal("LogMode 不应复用原实例")
	}
	if got.level != gormlogger.Info || got.slowThreshold != time.Second {
		t.Fatalf("LogMode 结果不符: %+v", got)
	}
	if l.level != gormlogger.Warn {
		t.Fatal("LogMode 不应修改原实例")
	}
}

// TestGormLog_TraceSkipsRecordNotFound 验证记录不存在属正常流，不读取 SQL（即不输出日志）
func TestGormLog_TraceSkipsRecordNotFound(t *testing.T) {
	t.Parallel()

	l := &gormLog{level: gormlogger.Warn, slowThreshold: time.Second}
	called := false
	l.Trace(context.Background(), time.Now(), func() (string, int64) {
		called = true
		return "SELECT 1", 0
	}, gorm.ErrRecordNotFound)
	if called {
		t.Fatal("记录不存在不应读取 SQL 并输出日志")
	}
}

// TestGormLog_TraceLogsError 验证真实错误仍会读取 SQL 并输出
func TestGormLog_TraceLogsError(t *testing.T) {
	t.Parallel()

	l := &gormLog{level: gormlogger.Warn, slowThreshold: time.Second}
	called := false
	l.Trace(context.Background(), time.Now(), func() (string, int64) {
		called = true
		return "SELECT 1", 0
	}, errors.New("boom"))
	if !called {
		t.Fatal("真实错误应读取 SQL 并输出日志")
	}
}

// TestGormLog_TraceLogsSlowSQL 验证超过阈值会读取 SQL 并输出慢 SQL 日志
func TestGormLog_TraceLogsSlowSQL(t *testing.T) {
	t.Parallel()

	l := &gormLog{level: gormlogger.Warn, slowThreshold: time.Nanosecond}
	called := false
	l.Trace(context.Background(), time.Now().Add(-time.Second), func() (string, int64) {
		called = true
		return "SELECT 1", 1
	}, nil)
	if !called {
		t.Fatal("慢 SQL 应读取 SQL 并输出日志")
	}
}
