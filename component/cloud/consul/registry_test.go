// Package consul 的服务注册单测
//
// @author zwei
package consul

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/consul/api"
)

// TestRegisterServiceUsesConfiguredDeregisterAfter 验证注册时按开关和配置写入注销时长
func TestRegisterServiceUsesConfiguredDeregisterAfter(t *testing.T) {
	t.Parallel()

	disabled := false
	tests := []struct {
		name    string
		enabled *bool
		seconds int
		want    string
	}{
		{name: "默认 30 分钟", seconds: 0, want: "30m0s"},
		{name: "自定义 10 分钟", seconds: 600, want: "10m0s"},
		{name: "关闭自动注销", enabled: &disabled, seconds: 600, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var registered api.AgentServiceRegistration
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/agent/service/register" {
					t.Errorf("请求路径期望 /v1/agent/service/register，实际 %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				if err := json.NewDecoder(r.Body).Decode(&registered); err != nil {
					t.Errorf("解析注册请求失败: %v", err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			apiClient, err := api.NewClient(&api.Config{
				Address:    server.URL,
				HttpClient: server.Client(),
			})
			if err != nil {
				t.Fatalf("创建 consul api 客户端失败: %v", err)
			}
			client := &Client{api: apiClient}
			cfg := Config{
				Addr:                   server.URL,
				ServiceID:              "svc-1",
				ServiceAddress:         "127.0.0.1:8080",
				CheckAddress:           "http://127.0.0.1:8080/health",
				DeregisterEnabled:      tt.enabled,
				DeregisterAfterSeconds: tt.seconds,
			}
			if _, err := client.RegisterService(cfg, "app"); err != nil {
				t.Fatalf("注册服务失败: %v", err)
			}
			if registered.Check == nil {
				t.Fatal("注册请求缺少健康检查配置")
			}
			if got := registered.Check.DeregisterCriticalServiceAfter; got != tt.want {
				t.Fatalf("注销时长期望 %q，实际 %q", tt.want, got)
			}
		})
	}
}

// TestRegisterServiceErrorPaths 验证注册前置校验与 Agent 失败的错误路径
func TestRegisterServiceErrorPaths(t *testing.T) {
	t.Parallel()

	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer agent.Close()

	apiClient, err := api.NewClient(&api.Config{Address: agent.URL, HttpClient: agent.Client()})
	if err != nil {
		t.Fatalf("创建 consul api 客户端失败: %v", err)
	}
	client := &Client{api: apiClient}

	validBase := Config{
		Addr:           agent.URL,
		ServiceID:      "svc-1",
		ServiceAddress: "127.0.0.1:8080",
		CheckAddress:   "http://127.0.0.1:8080/health",
	}
	tests := []struct {
		name    string
		mutate  func(cfg *Config)
		wantErr string
	}{
		{name: "addr 为空", mutate: func(cfg *Config) { cfg.Addr = " " }, wantErr: "consul 配置不能为空"},
		{name: "service_address 缺端口", mutate: func(cfg *Config) { cfg.ServiceAddress = "127.0.0.1" }, wantErr: "consul.service_address 无效"},
		{name: "check_address 非法协议", mutate: func(cfg *Config) { cfg.CheckAddress = "ftp://127.0.0.1/health" }, wantErr: "consul.check_address 无效"},
		{name: "Agent 注册失败", mutate: func(cfg *Config) {}, wantErr: "consul 注册失败"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := validBase
			tt.mutate(&cfg)
			if _, err := client.RegisterService(cfg, "app"); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("期望错误包含 %q，实际: %v", tt.wantErr, err)
			}
		})
	}
}

// TestDeregisterService 验证注销请求路径、空 ID 跳过与 Agent 失败报错
func TestDeregisterService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		serviceID  string
		agentCode  int
		wantPath   string
		wantCalled bool
		wantErr    string
	}{
		{name: "正常注销", serviceID: "svc-1", agentCode: http.StatusOK,
			wantPath: "/v1/agent/service/deregister/svc-1", wantCalled: true},
		{name: "Agent 失败", serviceID: "svc-1", agentCode: http.StatusInternalServerError,
			wantPath: "/v1/agent/service/deregister/svc-1", wantCalled: true, wantErr: "consul 注销失败"},
		{name: "空 ID 跳过请求", serviceID: "   ", wantCalled: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var called int32
			agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&called, 1)
				if tt.wantPath != "" && r.URL.Path != tt.wantPath {
					t.Errorf("请求路径期望 %s，实际 %s", tt.wantPath, r.URL.Path)
				}
				w.WriteHeader(tt.agentCode)
			}))
			defer agent.Close()

			apiClient, err := api.NewClient(&api.Config{Address: agent.URL, HttpClient: agent.Client()})
			if err != nil {
				t.Fatalf("创建 consul api 客户端失败: %v", err)
			}
			client := &Client{api: apiClient}

			err = client.DeregisterService(tt.serviceID)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("期望错误包含 %q，实际: %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("注销不应报错: %v", err)
			}
			if got := atomic.LoadInt32(&called); (got > 0) != tt.wantCalled {
				t.Fatalf("Agent 被调用=%d，期望 called=%v", got, tt.wantCalled)
			}
		})
	}
}
