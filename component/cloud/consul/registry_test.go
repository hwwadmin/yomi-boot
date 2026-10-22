// Package consul 的服务注册单测
//
// @author zwei
package consul

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
