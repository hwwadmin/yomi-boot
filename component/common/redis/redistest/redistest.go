// Package redistest 提供基于 Testcontainers 的 Redis 7.4 测试辅助。
//
// 每个测试进程共用一个 redis:7.4.11 容器，每个用例占用一个独立库号，用例结束时 FLUSHDB 并归还。
// 容器在测试进程退出时由 Testcontainers 回收。
//
// @author zwei
package redistest

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	redisv9 "github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

const (
	// Image 是测试容器使用的 Redis 镜像，与 README 推荐的线上版本一致
	Image = "redis:7.4.11"
	// Host 是测试代码连接容器时使用的地址。容器端口发布在本机。
	Host = "127.0.0.1"

	databaseCount  = 128
	startupTimeout = 2 * time.Minute
	acquireTimeout = 30 * time.Second
	commandTimeout = 10 * time.Second
)

// redisServer 是当前测试进程共用的 Redis 容器
type redisServer struct {
	addr string
	free chan int
}

var (
	startOnce sync.Once
	server    *redisServer
	startErr  error
)

// NewClient 返回连到独立库号的 go-redis 客户端与清理函数。
// 清理函数关闭客户端、清空该库并归还库号，不停止容器。
func NewClient(t testing.TB) (*redisv9.Client, func()) {
	t.Helper()

	addr, db, release := acquire(t)
	client := redisv9.NewClient(&redisv9.Options{Addr: addr, DB: db})
	cleanup := func() {
		_ = client.Close()
		release()
	}
	t.Cleanup(func() { _ = client.Close() })
	return client, cleanup
}

// NewServer 返回容器地址、独立库号与清理函数，供需要自行构建客户端的使用方使用。
// 调用方必须把 db 写入客户端配置，否则会连到 0 号库并和其他用例冲突。
func NewServer(t testing.TB) (addr string, db int, cleanup func()) {
	t.Helper()

	return acquire(t)
}

// acquire 领用一个库号。清理函数清空该库后归还；测试结束时也会自动归还。
func acquire(t testing.TB) (addr string, db int, release func()) {
	t.Helper()

	srv := ensureServer(t)
	select {
	case db = <-srv.free:
	case <-time.After(acquireTimeout):
		t.Fatal("redistest: 没有空闲的 Redis 库")
	}
	if err := flushDB(srv.addr, db); err != nil {
		srv.free <- db
		t.Fatalf("redistest: 清空 Redis 库 %d 失败: %v", db, err)
	}

	var once sync.Once
	release = func() {
		once.Do(func() {
			if err := flushDB(srv.addr, db); err != nil {
				t.Errorf("redistest: 归还前清空 Redis 库 %d 失败: %v", db, err)
			}
			srv.free <- db
		})
	}
	t.Cleanup(release)
	return srv.addr, db, release
}

// ensureServer 懒启动当前进程唯一的 Redis 容器
func ensureServer(t testing.TB) *redisServer {
	t.Helper()

	startOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), startupTimeout)
		defer cancel()

		ctr, err := tcredis.Run(ctx, Image,
			testcontainers.WithCmdArgs("--databases", fmt.Sprintf("%d", databaseCount)),
		)
		if err != nil {
			startErr = fmt.Errorf("启动 Redis 容器: %w", err)
			return
		}

		mapped, err := ctr.MappedPort(ctx, "6379/tcp")
		if err != nil {
			startErr = fmt.Errorf("读取 Redis 映射端口: %w", err)
			return
		}
		addr := fmt.Sprintf("%s:%d", Host, int(mapped.Num()))

		pingCtx, pingCancel := context.WithTimeout(context.Background(), commandTimeout)
		defer pingCancel()
		if err := pingReady(pingCtx, addr); err != nil {
			startErr = fmt.Errorf("等待 Redis 可连接: %w", err)
			return
		}

		free := make(chan int, databaseCount)
		for i := 0; i < databaseCount; i++ {
			free <- i
		}
		server = &redisServer{addr: addr, free: free}
	})
	if startErr != nil {
		t.Fatalf("redistest: 启动 Redis 容器失败: %v", startErr)
	}
	return server
}

// pingReady 在容器日志报就绪后继续探测
func pingReady(ctx context.Context, addr string) error {
	client := redisv9.NewClient(&redisv9.Options{Addr: addr})
	defer func() { _ = client.Close() }()

	var last error
	for {
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		last = client.Ping(pingCtx).Err()
		cancel()
		if last == nil {
			return nil
		}
		if ctx.Err() != nil {
			return last
		}
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return last
		case <-timer.C:
		}
	}
}

func flushDB(addr string, db int) error {
	client := redisv9.NewClient(&redisv9.Options{Addr: addr, DB: db})
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	return client.FlushDB(ctx).Err()
}
