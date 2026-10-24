// 数据源连接生命周期的单元测试
//
// @author zwei

package db

import (
	"sync"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/db/dbtest"
)

// TestConn_CloseIdempotent 验证 Conn.Close 可重复调用，且关闭后不再持有 GORM 句柄
func TestConn_CloseIdempotent(t *testing.T) {
	gormDB, cleanup := dbtest.New(t)
	defer cleanup()

	conn := &Conn{}
	conn.db.Store(gormDB)
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

// TestConn_CloseConcurrentWithGORM 验证 Close 与 GORM 并发调用无数据竞争（W-1 回归）。
// 竞争本身只有 go test -race 能检出，本用例为其载体；无 nil 检查的读方遵循
// 「关闭后 GORM 返回 nil」契约须自行判空，不构成本用例关注点
func TestConn_CloseConcurrentWithGORM(t *testing.T) {
	gormDB, cleanup := dbtest.New(t)
	defer cleanup()

	conn := &Conn{}
	conn.db.Store(gormDB)

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					if g := conn.GORM(); g != nil {
						_ = g.Statement
					}
				}
			}
		}()
	}
	time.Sleep(50 * time.Millisecond)
	if err := conn.Close(); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}
	close(stop)
	wg.Wait()
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
