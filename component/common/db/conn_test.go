// 数据源连接生命周期的单元测试
//
// @author zwei

package db

import (
	"testing"

	"gitee.com/hq_project/yomi-boot/component/common/db/dbtest"
)

// TestConn_CloseIdempotent 验证 Conn.Close 可重复调用，且关闭后不再持有 GORM 句柄
func TestConn_CloseIdempotent(t *testing.T) {
	gormDB, cleanup := dbtest.New(t)
	defer cleanup()

	conn := &Conn{db: gormDB}
	if err := conn.Close(); err != nil {
		t.Fatalf("首次关闭失败: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("重复关闭应返回 nil: %v", err)
	}
	if conn.GORM() != nil {
		t.Fatal("关闭后 GORM 应返回 nil")
	}
}

// TestConn_NilSafe 验证 nil Conn 的访问器与 Close 安全
func TestConn_NilSafe(t *testing.T) {
	var conn *Conn
	if conn.GORM() != nil {
		t.Fatal("nil Conn 的 GORM 应返回 nil")
	}
	if conn.Type() != "" {
		t.Fatal("nil Conn 的 Type 应返回空值")
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("nil Conn 关闭应返回 nil: %v", err)
	}
}
