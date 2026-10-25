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

// NewServer 启动内存 Redis 并返回地址与清理函数，供需要自行构建客户端的使用方使用
func NewServer(t testing.TB) (addr string, cleanup func()) {
	t.Helper()

	mr := miniredis.RunT(t)
	return mr.Addr(), func() { mr.Close() }
}
