// Package redistest 提供 Redis 集成测试辅助
//
// @author zwei
package redistest

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	redisv9 "github.com/redis/go-redis/v9"
)

// NewClient 启动内存 Redis 并返回 go-redis 客户端与清理函数
func NewClient(t testing.TB) (*redisv9.Client, func()) {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redisv9.NewClient(&redisv9.Options{Addr: mr.Addr()})
	cleanup := func() {
		_ = client.Close()
	}
	return client, cleanup
}
