// Package consultest 提供基于 Testcontainers 的 Consul 1.21 测试辅助。
//
// 每个测试进程共用一个 hashicorp/consul:1.21.5 容器。每个用例领一个服务名前缀，
// 结束时只注销该前缀下的实例，不停止容器。容器在测试进程退出时由 Testcontainers 回收。
//
// @author zwei
package consultest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/hwwadmin/yomi-boot/component/cloud/consul"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// Image 是测试容器使用的 Consul 镜像，与 README 推荐的线上版本一致。
	Image = "hashicorp/consul:1.21.5"
	// Host 是测试代码连接容器时使用的地址。容器端口发布在本机。
	Host = "127.0.0.1"

	clientPort     = "8500/tcp"
	startupTimeout = 3 * time.Minute
	commandTimeout = 10 * time.Second
	// discoveryCacheSeconds 让用例在注销后很快看到目录变化，不必等生产默认的 30 秒。
	discoveryCacheSeconds = 1
)

// consulServer 是当前测试进程共用的 Consul Agent。
type consulServer struct {
	addr   string
	client *api.Client
	mu     sync.Mutex
}

var (
	startOnce sync.Once
	server    *consulServer
	startErr  error
)

// NewClient 返回指向共用 Agent 的客户端、本用例服务名前缀和清理函数。
// 发现缓存为 1 秒，并关闭陈旧兜底。清理函数只注销带该前缀的实例，不停止容器。
func NewClient(t testing.TB) (*consul.Client, string, func()) {
	t.Helper()

	srv := ensureServer(t)
	prefix, err := newPrefix()
	if err != nil {
		t.Fatalf("consultest: 生成服务名前缀失败: %v", err)
	}
	stale := 0
	client, err := consul.NewClient(consul.Config{
		Addr:                         srv.addr,
		DiscoveryCacheSeconds:        discoveryCacheSeconds,
		DiscoveryStaleIfErrorSeconds: &stale,
		HTTPTimeoutSeconds:           int(commandTimeout / time.Second),
	})
	if err != nil {
		t.Fatalf("consultest: 创建 Consul 客户端失败: %v", err)
	}

	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			if err := srv.release(prefix); err != nil {
				t.Errorf("consultest: 清理前缀 %s 失败: %v", prefix, err)
			}
		})
	}
	t.Cleanup(cleanup)
	return client, prefix, cleanup
}

// Register 向共用 Agent 注册一个没有健康检查的实例。
// 服务名和实例 ID 都会带上 prefix。没有检查的实例视为健康，可以被发现。
func Register(t testing.TB, prefix, name, host string, port int) (serviceName, serviceID string) {
	t.Helper()

	prefix = strings.TrimSpace(prefix)
	name = strings.TrimSpace(name)
	host = strings.TrimSpace(host)
	if prefix == "" || name == "" || host == "" || port <= 0 {
		t.Fatalf("consultest: 注册参数无效 prefix=%q name=%q host=%q port=%d", prefix, name, host, port)
	}
	serviceName = prefix + name
	serviceID = serviceName

	srv := ensureServer(t)
	srv.mu.Lock()
	defer srv.mu.Unlock()
	err := srv.client.Agent().ServiceRegister(&api.AgentServiceRegistration{
		ID:      serviceID,
		Name:    serviceName,
		Address: host,
		Port:    port,
	})
	if err != nil {
		t.Fatalf("consultest: 注册 %s 失败: %v", serviceName, err)
	}
	return serviceName, serviceID
}

// Deregister 按实例 ID 注销服务。实例已经不存在时视为成功。
func Deregister(t testing.TB, serviceID string) {
	t.Helper()

	serviceID = strings.TrimSpace(serviceID)
	if serviceID == "" {
		t.Fatal("consultest: serviceID 不能为空")
	}
	srv := ensureServer(t)
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if err := srv.deregister(serviceID); err != nil {
		t.Fatalf("consultest: 注销 %s 失败: %v", serviceID, err)
	}
}

// ensureServer 懒启动当前进程唯一的 Consul 容器。
func ensureServer(t testing.TB) *consulServer {
	t.Helper()

	startOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), startupTimeout)
		defer cancel()

		ctr, err := testcontainers.Run(ctx, Image,
			testcontainers.WithExposedPorts(clientPort),
			testcontainers.WithWaitStrategyAndDeadline(startupTimeout, leaderReady()),
		)
		if err != nil {
			startErr = fmt.Errorf("启动 Consul 容器: %w", err)
			return
		}

		mapped, err := ctr.MappedPort(ctx, clientPort)
		if err != nil {
			startErr = fmt.Errorf("读取 Consul 映射端口: %w", err)
			return
		}
		addr := fmt.Sprintf("%s:%d", Host, int(mapped.Num()))
		client, err := api.NewClient(&api.Config{
			Address:    addr,
			Scheme:     "http",
			HttpClient: &http.Client{Timeout: commandTimeout},
		})
		if err != nil {
			startErr = fmt.Errorf("创建 Consul API 客户端: %w", err)
			return
		}
		if err := pingReady(ctx, client); err != nil {
			startErr = fmt.Errorf("等待 Consul 选出 leader: %w", err)
			return
		}
		server = &consulServer{addr: addr, client: client}
	})
	if startErr != nil {
		t.Fatalf("consultest: 启动 Consul 容器失败: %v", startErr)
	}
	return server
}

// leaderReady 等待开发模式 Agent 选出 leader。空 leader 表示还不能接受目录写入。
func leaderReady() *wait.HTTPStrategy {
	return wait.ForHTTP("/v1/status/leader").
		WithPort(clientPort).
		WithStartupTimeout(startupTimeout).
		WithForcedIPv4LocalHost().
		WithStatusCodeMatcher(func(code int) bool { return code == http.StatusOK }).
		WithResponseMatcher(func(body io.Reader) bool {
			raw, err := io.ReadAll(body)
			if err != nil {
				return false
			}
			leader := strings.Trim(string(raw), "\" \t\r\n")
			return leader != ""
		})
}

func pingReady(ctx context.Context, client *api.Client) error {
	var last error
	for {
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		leader, err := client.Status().LeaderWithQueryOptions((&api.QueryOptions{}).WithContext(pingCtx))
		cancel()
		if err == nil && strings.TrimSpace(leader) != "" {
			return nil
		}
		last = err
		if last == nil {
			last = fmt.Errorf("leader 为空")
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

// release 注销本用例前缀下的实例。其他用例和 Consul 自身服务不动。
func (s *consulServer) release(prefix string) error {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	s.mu.Lock()
	defer s.mu.Unlock()

	services, err := s.client.Agent().ServicesWithFilterOpts("", (&api.QueryOptions{}).WithContext(ctx))
	if err != nil {
		return err
	}
	var errs []error
	for id, svc := range services {
		if !ownedByPrefix(prefix, id, svc) {
			continue
		}
		if err := s.deregister(id); err != nil {
			errs = append(errs, err)
		}
	}
	return joinErrs(errs)
}

func (s *consulServer) deregister(serviceID string) error {
	err := s.client.Agent().ServiceDeregister(serviceID)
	if err == nil || isMissing(err) {
		return nil
	}
	return fmt.Errorf("注销 %s: %w", serviceID, err)
}

func ownedByPrefix(prefix, id string, svc *api.AgentService) bool {
	if strings.HasPrefix(id, prefix) {
		return true
	}
	return svc != nil && strings.HasPrefix(svc.Service, prefix)
}

func isMissing(err error) bool {
	return err != nil && strings.Contains(err.Error(), "404")
}

func newPrefix() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return "yomi" + hex.EncodeToString(buf[:]) + "-", nil
}

func joinErrs(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	parts := make([]string, len(errs))
	for i, err := range errs {
		parts[i] = err.Error()
	}
	return fmt.Errorf("%s", strings.Join(parts, "; "))
}
