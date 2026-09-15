// Package aiclient 按填写的实网配置调用门面
//
// @author zwei
package aiclient

import (
	"context"
	"strings"
	"testing"
	"time"
)

// testConfig 实网 OpenAI 兼容配置；测前填 APIURL、APIKey、Model
var testConfig = Config{
	Protocol: ProtocolOpenAI,
	APIURL:   "https://api.deepseek.com",
	APIKey:   "",
	Model:    "deepseek-flash",
	Timeout:  60 * time.Second,
}

// testConfig2 实网 Anthropic 兼容配置；测前填 APIURL、APIKey、Model
var testConfig2 = Config{
	Protocol: ProtocolAnthropic,
	APIURL:   "https://api.deepseek.com/anthropic",
	APIKey:   "",
	Model:    "deepseek-flash",
	Timeout:  60 * time.Second,
}

// requireLiveConfig 校验实网配置已填密钥与地址；未填则跳过当前子测试
func requireLiveConfig(t *testing.T, cfg Config) Config {
	t.Helper()
	if strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.APIURL) == "" {
		t.Skip("填写对应配置的 APIURL 与 APIKey 后再跑")
	}
	return cfg
}

// TestChat 分别用 OpenAI 与 Anthropic 两套配置发起同步 Chat
func TestChat(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{"openai", testConfig},
		{"anthropic", testConfig2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cli, err := New(requireLiveConfig(t, tc.cfg))
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			resp, err := cli.Chat(context.Background(), ChatRequest{
				Messages: []Message{{Role: RoleUser, Content: "ping"}},
			})
			if err != nil {
				t.Fatalf("Chat: %v", err)
			}
			if resp == nil || resp.Message.Role != RoleAssistant || strings.TrimSpace(resp.Message.Content) == "" {
				t.Fatalf("resp=%+v", resp)
			}
			t.Logf("protocol=%s model=%s content=%q", tc.cfg.Protocol, resp.Model, resp.Message.Content)
		})
	}
}
