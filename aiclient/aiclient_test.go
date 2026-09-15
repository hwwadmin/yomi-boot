// Package aiclient 按填写的 testConfig 实网调用门面
//
// @author zwei
package aiclient

import (
	"context"
	"strings"
	"testing"
	"time"
)

// testConfig 实网测试配置；测前填 APIURL、APIKey、DefaultModel
//
// Protocol 取 ProtocolOpenAI 或 ProtocolAnthropic。APIKey 为空则跳过。示例：
//   - openai: ProtocolOpenAI, APIURL https://api.openai.com/v1
//   - anthropic: ProtocolAnthropic, APIURL https://api.anthropic.com
var testConfig = Config{
	Protocol:     ProtocolOpenAI,
	APIURL:       "https://api.openai.com/v1",
	APIKey:       "",
	DefaultModel: "gpt-4o-mini",
	Timeout:      60 * time.Second,
}

// requireTestConfig 返回已填写的测试配置；未填密钥或地址则跳过
func requireTestConfig(t *testing.T) Config {
	t.Helper()
	cfg := testConfig
	if strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.APIURL) == "" {
		t.Skip("填写 testConfig 的 APIURL 与 APIKey 后再跑")
	}
	return cfg
}

// TestChat 用 testConfig 构造 AIClient 并发起同步 Chat
func TestChat(t *testing.T) {
	cli, err := New(requireTestConfig(t))
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
	t.Logf("protocol=%s model=%s content=%q", testConfig.Protocol, resp.Model, resp.Message.Content)
}
