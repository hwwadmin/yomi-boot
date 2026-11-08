package redisstream

import (
	"testing"

	redisv9 "github.com/redis/go-redis/v9"
)

func TestNewComponents_InvalidConfig(t *testing.T) {
	client := redisv9.NewClient(&redisv9.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { _ = client.Close() })
	if _, err := NewComponents(Config{BlockMS: -1}, client); err == nil {
		t.Fatal("非法配置应失败")
	}
}
