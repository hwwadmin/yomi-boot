// Package dtmtest 提供基于 Testcontainers 的 DTM 1.19 测试辅助。
//
// 每个测试进程共用一对容器：redis:7.4.11 和 yedf/dtm:1.19.0。DTM 只使用这台 Redis，
// 不连接本机 MySQL，也不复用 dbtest、redistest 的容器。事务 ID 由 DTM 生成，用例之间靠 gid 隔离，
// 清理函数不停止容器，也不清空 Redis。容器在测试进程退出时由 Testcontainers 回收。
//
// @author zwei
package dtmtest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/hwwadmin/yomi-boot/component/cloud/dtm"
	"github.com/moby/moby/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// Image 是测试容器使用的 DTM 镜像，与 README 推荐的线上版本一致。
	Image = "yedf/dtm:1.19.0"
	// RedisImage 是 DTM 专用的 Redis 镜像，与 README 推荐的线上版本一致。
	RedisImage = "redis:7.4.11"
	// Host 是测试代码连接 DTM 时使用的地址。容器端口发布在本机。
	Host = "127.0.0.1"
	// CallbackHost 是容器访问测试进程时使用的主机名。
	CallbackHost = "host.docker.internal"

	httpPort       = "36789/tcp"
	redisAlias     = "redis"
	startupTimeout = 3 * time.Minute
	clientTimeout  = 15 * time.Second
)

// dtmServer 是当前测试进程共用的 DTM HTTP 入口。
type dtmServer struct {
	apiBase string
}

var (
	startOnce sync.Once
	server    *dtmServer
	startErr  error
)

// NewClient 返回指向共用 DTM 的客户端和清理函数。
// Consul 为空。清理函数可重复调用，不停止容器，也不清空事务数据。
func NewClient(t testing.TB) (*dtm.Client, func()) {
	t.Helper()

	srv := ensureServer(t)
	client := dtm.NewClient(nil, srv.apiBase, dtm.WithTimeout(clientTimeout))
	var once sync.Once
	cleanup := func() { once.Do(func() {}) }
	t.Cleanup(cleanup)
	return client, cleanup
}

// CallbackBase 返回容器访问测试进程的基址。port 是测试进程在 0.0.0.0 上监听的端口。
func CallbackBase(port int) string {
	return fmt.Sprintf("http://%s:%d", CallbackHost, port)
}

// ensureServer 懒启动当前进程唯一的 Redis 与 DTM。
func ensureServer(t testing.TB) *dtmServer {
	t.Helper()

	startOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), startupTimeout)
		defer cancel()

		nw, err := network.New(ctx)
		if err != nil {
			startErr = fmt.Errorf("创建 Docker 网络: %w", err)
			return
		}
		_, err = tcredis.Run(ctx, RedisImage,
			network.WithNetwork([]string{redisAlias}, nw),
			testcontainers.WithWaitStrategyAndDeadline(startupTimeout,
				wait.ForLog("Ready to accept connections").WithStartupTimeout(startupTimeout),
			),
		)
		if err != nil {
			startErr = fmt.Errorf("启动 Redis 容器: %w", err)
			return
		}

		ctr, err := testcontainers.Run(ctx, Image,
			testcontainers.WithExposedPorts(httpPort),
			testcontainers.WithEnv(map[string]string{
				"STORE_DRIVER": "redis",
				"STORE_HOST":   redisAlias,
				"STORE_PORT":   "6379",
			}),
			network.WithNetwork([]string{"dtm"}, nw),
			testcontainers.WithHostConfigModifier(func(hc *container.HostConfig) {
				hc.ExtraHosts = append(hc.ExtraHosts, CallbackHost+":host-gateway")
			}),
			testcontainers.WithWaitStrategyAndDeadline(startupTimeout, versionReady()),
		)
		if err != nil {
			startErr = fmt.Errorf("启动 DTM 容器: %w", err)
			return
		}

		mapped, err := ctr.MappedPort(ctx, httpPort)
		if err != nil {
			startErr = fmt.Errorf("读取 DTM 映射端口: %w", err)
			return
		}
		apiBase := fmt.Sprintf("http://%s:%d/api/dtmsvr", Host, int(mapped.Num()))
		if err := pingVersion(ctx, apiBase); err != nil {
			startErr = fmt.Errorf("等待 DTM 可连接: %w", err)
			return
		}
		server = &dtmServer{apiBase: apiBase}
	})
	if startErr != nil {
		t.Fatalf("dtmtest: 启动 DTM 失败: %v", startErr)
	}
	return server
}

// versionReady 等待 HTTP API 开始响应。存储未就绪时 DTM 还不会监听该端口。
func versionReady() *wait.HTTPStrategy {
	return wait.ForHTTP("/api/dtmsvr/version").
		WithPort(httpPort).
		WithStartupTimeout(startupTimeout).
		WithForcedIPv4LocalHost().
		WithStatusCodeMatcher(func(code int) bool { return code == http.StatusOK })
}

func pingVersion(ctx context.Context, apiBase string) error {
	client := &http.Client{Timeout: 3 * time.Second}
	var last error
	for {
		reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, apiBase+"/version", nil)
		if err != nil {
			cancel()
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				cancel()
				return nil
			}
			err = fmt.Errorf("status %d", resp.StatusCode)
		}
		cancel()
		last = err
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
