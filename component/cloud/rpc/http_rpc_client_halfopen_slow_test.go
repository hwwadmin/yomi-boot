package rpc

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hwwadmin/yomi-boot/component/core/errors"
	"github.com/hwwadmin/yomi-boot/component/core/logger"
)

// waitCooldownEnded 轮询等待熔断冷却计时结束；allow 会占用唯一探测位，不能用它做等待探测
func waitCooldownEnded(c *HttpRpcClient, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		c.breaker.mu.Lock()
		openUntil := c.breaker.openUntil
		c.breaker.mu.Unlock()
		if time.Now().After(openUntil) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// waitLoggedLine deadline 轮询等待日志目录里出现同时包含全部片段的日志行，超时直接 Fatal
func waitLoggedLine(t *testing.T, dir string, timeout time.Duration, fragments ...string) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		line, found := findLoggedLine(dir, fragments...)
		if found {
			return line
		}
		if time.Now().After(deadline) {
			t.Fatalf("日志中未找到预期行, fragments=%v", fragments)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// findLoggedLine 遍历日志目录所有文件，返回同时包含全部片段的第一条日志行
func findLoggedLine(dir string, fragments ...string) (string, bool) {
	var found string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		for _, line := range strings.Split(string(data), "\n") {
			matched := true
			for _, fragment := range fragments {
				if !strings.Contains(line, fragment) {
					matched = false
					break
				}
			}
			if matched {
				found = line
				return filepath.SkipAll
			}
		}
		return nil
	})
	return found, found != ""
}

// TestInvoke_HalfOpenSlowProbeReopensWithSlowReason 验证半开探测请求 HTTP 成功但超过慢调用阈值：
// 请求本身成功返回，熔断以 reason=slow 重新打开，后续调用被重新拦截
func TestInvoke_HalfOpenSlowProbeReopensWithSlowReason(t *testing.T) {
	// reason 只随 rpc.breaker.open 日志可观察：初始化独立日志目录（同步落盘、关控制台）用于断言
	consoleEnabled, asyncEnabled := false, false
	logDir := t.TempDir()
	stopLogger, err := logger.InitLogger("rpc-breaker-log-test", "test", logger.Config{
		Dir:            logDir,
		Level:          "info",
		ConsoleEnabled: &consoleEnabled,
		AsyncEnabled:   &asyncEnabled,
		Rotation:       logger.RotationConfig{MaxSizeMB: 1, RetentionDays: 1, MaxBackups: 2, MaxTotalMB: 10},
	}, nil)
	if err != nil {
		t.Fatalf("初始化测试日志失败: %v", err)
	}
	defer func() { _ = stopLogger() }()

	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		time.Sleep(80 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"OK"}`))
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL,
		WithBreaker(BreakerOptions{
			WindowSize: 1, MinRequests: 1, FailureRate: 1, SlowRate: 1,
			SlowThreshold: 20 * time.Millisecond, OpenDuration: 30 * time.Millisecond,
		}),
	)
	var env testEnvelope

	// 首次调用 500 打开熔断（该路径日志 reason=http，与探测重新打开的 slow 区分开）
	if err := invokeTestWith(client, context.Background(), "/x", nil, &env, 1); err == nil {
		t.Fatal("首次 500 应返回错误")
	}
	if state, _ := breakerSnapshot(client); state != breakerOpen {
		t.Fatalf("首次 500 应打开熔断，state=%d", state)
	}

	// deadline 轮询等冷却结束进入半开
	if !waitCooldownEnded(client, 2*time.Second) {
		t.Fatal("等待半开超时")
	}

	// 半开探测：HTTP 200 反序列化成功，但耗时超过慢调用阈值
	before := hits.Load()
	if err := client.Invoke(context.Background(), InvokeRequest{
		Endpoint: "/x", RespBody: &env, Timeout: 500 * time.Millisecond, MaxAttempts: 1,
	}); err != nil {
		t.Fatalf("半开慢探测请求本身应成功返回: %v", err)
	}
	if env.Code != "OK" {
		t.Fatalf("探测响应应已反序列化, code=%q", env.Code)
	}
	if got := hits.Load() - before; got != 1 {
		t.Fatalf("半开探测只应发一次请求, hits delta=%d", got)
	}
	if state, _ := breakerSnapshot(client); state != breakerOpen {
		t.Fatalf("慢探测应重新打开熔断, state=%d", state)
	}
	if err := invokeTest(client, context.Background(), "/x", nil, &env); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("重新打开后应拦截后续调用, err=%v", err)
	}

	// 熔断重新打开日志应携带 reason=slow 与本次端点
	line := waitLoggedLine(t, logDir, 2*time.Second, `"message":"rpc.breaker.open"`, `"reason":"slow"`, client.serviceName)
	if !strings.Contains(line, `"endpoint":"/x"`) {
		t.Fatalf("重新打开日志应携带端点, line=%s", line)
	}
}
