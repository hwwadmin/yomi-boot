// Package consul 的服务发现主路径单测：Agent 查询、实例过滤、缓存与 URL 拼接
//
// 用 httptest 伪装 Consul Agent 的 /v1/health/service/{name} 接口。

package consul

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// newHealthAgent 伪装 Consul Agent 健康查询，entriesByService 为服务名到响应 JSON 的映射，
// 返回测试服务与统计各服务查询次数的函数
func newHealthAgent(t *testing.T, entriesByService map[string]string) (*httptest.Server, func(service string) int) {
	t.Helper()

	var mu sync.Mutex
	hits := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/v1/health/service/")
		mu.Lock()
		hits[name]++
		mu.Unlock()
		w.Header().Set("X-Consul-Index", "1")
		w.Header().Set("Content-Type", "application/json")
		entries, ok := entriesByService[name]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(entries))
	}))
	return server, func(service string) int {
		mu.Lock()
		defer mu.Unlock()
		return hits[service]
	}
}

// newDiscoveryClient 构造指向伪装 Agent 的发现客户端
func newDiscoveryClient(t *testing.T, agentURL string, discoveryURLScheme string) *Client {
	t.Helper()
	client, err := NewClient(Config{
		Addr:               agentURL,
		DiscoveryURLScheme: discoveryURLScheme,
	})
	if err != nil {
		t.Fatalf("创建发现客户端失败: %v", err)
	}
	return client
}

// TestResolveHTTPBaseFromAgent 验证 Agent 查询、无效实例过滤、Node 地址回退与一级缓存命中
func TestResolveHTTPBaseFromAgent(t *testing.T) {
	t.Parallel()

	// 实例依次为：Service.Address 生效、Node.Address 回退、无地址被过滤、端口非法被过滤
	agent, hits := newHealthAgent(t, map[string]string{
		"svc": `[` +
			`{"Node":{"Address":"10.0.0.9"},"Service":{"Address":"10.0.0.1","Port":8080}},` +
			`{"Node":{"Address":"10.0.0.2"},"Service":{"Address":"","Port":8081}},` +
			`{"Node":{"Address":""},"Service":{"Address":"","Port":8082}},` +
			`{"Node":{"Address":"10.0.0.4"},"Service":{"Address":"10.0.0.4","Port":0}}` +
			`]`,
	})
	defer agent.Close()

	client := newDiscoveryClient(t, agent.URL, "http://")

	first, err := client.ResolveHTTPBase(context.Background(), "svc", "/action")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if first != "http://10.0.0.1:8080/action" {
		t.Fatalf("首个实例期望 http://10.0.0.1:8080/action，实际 %s", first)
	}

	// 第二次解析命中一级缓存（Agent 只被查询一次），轮询计数器推进到下一个实例
	second, err := client.ResolveHTTPBase(context.Background(), "svc", "/action")
	if err != nil {
		t.Fatalf("缓存解析失败: %v", err)
	}
	if second != "http://10.0.0.2:8081/action" {
		t.Fatalf("第二次解析期望轮询到 http://10.0.0.2:8081/action，实际 %s", second)
	}
	if got := hits("svc"); got != 1 {
		t.Fatalf("第二次解析应命中一级缓存，Agent 查询次数期望 1，实际 %d", got)
	}
}

// TestResolveHTTPBaseErrors 验证发现路径的各类错误
func TestResolveHTTPBaseErrors(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name    string
		entries string
		service string
		ctx     context.Context
		wantErr string
	}{
		{name: "上下文已取消", entries: `[]`, service: "svc", ctx: ctx, wantErr: "context canceled"},
		{name: "服务名为空", entries: `[]`, service: "  ", ctx: context.Background(), wantErr: "服务名不能为空"},
		{name: "无健康实例", entries: `[]`, service: "svc", ctx: context.Background(), wantErr: "consul 未发现健康实例"},
		{name: "实例地址全部无效", entries: `[{"Node":{"Address":""},"Service":{"Address":"","Port":8080}}]`,
			service: "svc", ctx: context.Background(), wantErr: "consul 实例地址无效"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			agent, _ := newHealthAgent(t, map[string]string{"svc": tt.entries})
			defer agent.Close()

			client := newDiscoveryClient(t, agent.URL, "")
			_, err := client.ResolveHTTPBase(tt.ctx, tt.service, "/action")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("期望错误包含 %q，实际: %v", tt.wantErr, err)
			}
		})
	}
}

// TestFormatHTTPBase 验证实例地址与路径后缀的拼接规则
func TestFormatHTTPBase(t *testing.T) {
	t.Parallel()

	instance := ServiceInstance{Address: "10.0.0.1", Port: 8080}
	tests := []struct {
		name       string
		scheme     string
		pathSuffix string
		want       string
	}{
		{name: "无协议不拼接", scheme: "", pathSuffix: "/action", want: "10.0.0.1:8080/action"},
		{name: "带协议", scheme: "http://", pathSuffix: "/action", want: "http://10.0.0.1:8080/action"},
		{name: "后缀补斜杠", scheme: "http://", pathSuffix: "action", want: "http://10.0.0.1:8080/action"},
		{name: "后缀去尾部斜杠", scheme: "http://", pathSuffix: "action/", want: "http://10.0.0.1:8080/action"},
		{name: "空后缀", scheme: "http://", pathSuffix: "", want: "http://10.0.0.1:8080"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := &Client{discoveryURLScheme: tt.scheme}
			if got := client.formatHTTPBase(instance, tt.pathSuffix); got != tt.want {
				t.Fatalf("formatHTTPBase 期望 %q，实际 %q", tt.want, got)
			}
		})
	}
}

// TestInstancesRefreshAfterCacheExpiry 验证一级缓存过期后重新查询 Agent
func TestInstancesRefreshAfterCacheExpiry(t *testing.T) {
	t.Parallel()

	agent, hits := newHealthAgent(t, map[string]string{
		"svc": `[{"Node":{"Address":"10.0.0.1"},"Service":{"Address":"","Port":8080}}]`,
	})
	defer agent.Close()

	// 缓存 TTL 压到 30ms 以验证过期刷新（整型秒数放不下，直接白盒改写）；
	// 关闭陈旧兜底，保持"过期即同步重查"的原语义
	client, err := NewClient(Config{Addr: agent.URL})
	if err != nil {
		t.Fatalf("创建客户端失败: %v", err)
	}
	client.cacheTTL = 30 * time.Millisecond
	client.l1Cache = newServiceCache(client.cacheTTL)
	client.staleIfError = 0

	if _, err := client.ResolveHTTPBase(context.Background(), "svc", ""); err != nil {
		t.Fatalf("首次解析失败: %v", err)
	}
	time.Sleep(80 * time.Millisecond)
	if _, err := client.ResolveHTTPBase(context.Background(), "svc", ""); err != nil {
		t.Fatalf("过期后解析失败: %v", err)
	}
	if got := hits("svc"); got != 2 {
		t.Fatalf("缓存过期后应重新查询 Agent，期望 2 次，实际 %d 次", got)
	}
}

// switchableAgent 是可动态切换响应的伪装 Agent：entries 可改、fail 置 true 返回 500、latency 拉长响应
type switchableAgent struct {
	mu      sync.Mutex
	entries string
	fail    bool
	latency time.Duration
}

// newSwitchableAgent 启动可切换 Agent，返回服务、状态与查询计数
func newSwitchableAgent(t *testing.T, entries string) (*httptest.Server, *switchableAgent, func() int) {
	t.Helper()

	state := &switchableAgent{entries: entries}
	var mu sync.Mutex
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		fail, latency, body := state.fail, state.latency, state.entries
		state.mu.Unlock()
		mu.Lock()
		hits++
		mu.Unlock()
		if latency > 0 {
			time.Sleep(latency)
		}
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("X-Consul-Index", "1")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	return server, state, func() int {
		mu.Lock()
		defer mu.Unlock()
		return hits
	}
}

// newResilientClient 构造短 TTL / 短负缓存 / 指定兜底窗口的发现客户端（白盒改写，测试专用）
func newResilientClient(t *testing.T, agentURL string, cacheTTL, negTTL, stale time.Duration) *Client {
	t.Helper()
	client, err := NewClient(Config{Addr: agentURL})
	if err != nil {
		t.Fatalf("创建客户端失败: %v", err)
	}
	client.cacheTTL = cacheTTL
	client.l1Cache = newServiceCache(cacheTTL)
	client.negCache = newNegativeCache(negTTL)
	client.staleIfError = stale
	return client
}

// TestInstancesSingleflightMergesConcurrentMisses 验证并发 miss 只查询 Agent 一次
func TestInstancesSingleflightMergesConcurrentMisses(t *testing.T) {
	t.Parallel()

	agent, state, hits := newSwitchableAgent(t,
		`[{"Node":{"Address":"10.0.0.1"},"Service":{"Address":"10.0.0.1","Port":8080}}]`)
	defer agent.Close()
	state.mu.Lock()
	state.latency = 150 * time.Millisecond
	state.mu.Unlock()

	client := newResilientClient(t, agent.URL, time.Minute, time.Second, time.Minute)

	const n = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := client.ResolveHTTPBase(context.Background(), "svc", "/a"); err != nil {
				t.Errorf("并发解析失败: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := hits(); got != 1 {
		t.Fatalf("并发 miss 应合并为一次 Agent 查询，实际 %d 次", got)
	}
}

// TestInstancesServeStaleOnAgentFailure 验证 Agent 故障时陈旧兜底立即返回且探测被限频
func TestInstancesServeStaleOnAgentFailure(t *testing.T) {
	t.Parallel()

	agent, state, hits := newSwitchableAgent(t,
		`[{"Node":{"Address":"10.0.0.1"},"Service":{"Address":"10.0.0.1","Port":8080}}]`)
	defer agent.Close()

	client := newResilientClient(t, agent.URL, 30*time.Millisecond, 60*time.Millisecond, 5*time.Second)

	first, err := client.ResolveHTTPBase(context.Background(), "svc", "/a")
	if err != nil || first != "10.0.0.1:8080/a" {
		t.Fatalf("首次解析期望 10.0.0.1:8080/a，实际 %q err=%v", first, err)
	}

	time.Sleep(60 * time.Millisecond) // 等缓存过期
	state.mu.Lock()
	state.fail = true
	state.mu.Unlock()

	begin := time.Now()
	for i := 0; i < 3; i++ {
		got, err := client.ResolveHTTPBase(context.Background(), "svc", "/a")
		if err != nil || got != "10.0.0.1:8080/a" {
			t.Fatalf("Agent 故障期间应兜底返回陈旧实例，实际 %q err=%v", got, err)
		}
	}
	if elapsed := time.Since(begin); elapsed > time.Second {
		t.Fatalf("陈旧兜底应立即返回，实际耗时 %s", elapsed)
	}

	time.Sleep(150 * time.Millisecond) // 等后台再验证完成
	if got := hits(); got < 2 || got > 3 {
		t.Fatalf("故障期间探测应被限频（首查 1 次 + 再验证 1~2 次），实际 %d 次", got)
	}
}

// TestStalePurgedWhenNoHealthyInstances 验证"确认无健康实例"的确定性结果不做陈旧兜底
func TestStalePurgedWhenNoHealthyInstances(t *testing.T) {
	t.Parallel()

	agent, state, hits := newSwitchableAgent(t,
		`[{"Node":{"Address":"10.0.0.1"},"Service":{"Address":"10.0.0.1","Port":8080}}]`)
	defer agent.Close()

	client := newResilientClient(t, agent.URL, 30*time.Millisecond, 60*time.Millisecond, 5*time.Second)

	if _, err := client.ResolveHTTPBase(context.Background(), "svc", "/a"); err != nil {
		t.Fatalf("首次解析失败: %v", err)
	}
	time.Sleep(60 * time.Millisecond) // 等缓存过期
	state.mu.Lock()
	state.entries = "[]"
	state.mu.Unlock()

	// 再验证确认无健康实例前，允许一次性陈旧兜底
	if got, err := client.ResolveHTTPBase(context.Background(), "svc", "/a"); err != nil || got != "10.0.0.1:8080/a" {
		t.Fatalf("再验证完成前应兜底返回陈旧实例，实际 %q err=%v", got, err)
	}

	// 轮询等待后台再验证清除陈旧条目
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, err := client.ResolveHTTPBase(context.Background(), "svc", "/a")
		if err != nil {
			if !errors.Is(err, errNoHealthyInstances) {
				t.Fatalf("期望 errNoHealthyInstances，实际 %v", err)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("再验证确认无健康实例后应快速失败，不应持续陈旧兜底")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := hits(); got < 2 {
		t.Fatalf("再验证应至少查询一次 Agent，实际 %d 次", got)
	}
}

// TestNegativeCacheLimitsProbes 验证无陈旧可用时断连期间的失败负缓存
func TestNegativeCacheLimitsProbes(t *testing.T) {
	t.Parallel()

	agent, state, hits := newSwitchableAgent(t, `[]`)
	defer agent.Close()
	state.mu.Lock()
	state.fail = true
	state.mu.Unlock()

	client := newResilientClient(t, agent.URL, 30*time.Millisecond, 60*time.Millisecond, 5*time.Second)

	for i := 0; i < 3; i++ {
		if _, err := client.ResolveHTTPBase(context.Background(), "svc", "/a"); err == nil {
			t.Fatal("Agent 全 500 时应返回错误")
		}
	}
	if got := hits(); got != 1 {
		t.Fatalf("负缓存窗口内不应重复探测，期望 1 次，实际 %d 次", got)
	}

	time.Sleep(80 * time.Millisecond) // 等负缓存过期
	if _, err := client.ResolveHTTPBase(context.Background(), "svc", "/a"); err == nil {
		t.Fatal("负缓存过期后仍应失败")
	}
	if got := hits(); got != 2 {
		t.Fatalf("负缓存过期后应重新探测一次，期望 2 次，实际 %d 次", got)
	}
}

// TestRevalidateRecoversAfterOutage 验证故障恢复后后台再验证自动刷新实例列表
func TestRevalidateRecoversAfterOutage(t *testing.T) {
	t.Parallel()

	agent, state, _ := newSwitchableAgent(t,
		`[{"Node":{"Address":"10.0.0.1"},"Service":{"Address":"10.0.0.1","Port":8080}}]`)
	defer agent.Close()

	client := newResilientClient(t, agent.URL, 30*time.Millisecond, 60*time.Millisecond, 5*time.Second)

	if _, err := client.ResolveHTTPBase(context.Background(), "svc", "/a"); err != nil {
		t.Fatalf("首次解析失败: %v", err)
	}
	time.Sleep(60 * time.Millisecond) // 等缓存过期
	state.mu.Lock()
	state.fail = true
	state.mu.Unlock()
	if got, err := client.ResolveHTTPBase(context.Background(), "svc", "/a"); err != nil || got != "10.0.0.1:8080/a" {
		t.Fatalf("故障期间应兜底返回陈旧实例，实际 %q err=%v", got, err)
	}

	state.mu.Lock()
	state.fail = false
	state.entries = `[{"Node":{"Address":"10.0.0.2"},"Service":{"Address":"10.0.0.2","Port":8081}}]`
	state.mu.Unlock()

	deadline := time.Now().Add(2 * time.Second)
	for {
		got, err := client.ResolveHTTPBase(context.Background(), "svc", "/a")
		if err == nil && got == "10.0.0.2:8081/a" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("恢复后应自动刷新到新实例，最后结果 %q err=%v", got, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
