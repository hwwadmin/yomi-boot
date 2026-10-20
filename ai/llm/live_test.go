// Package llm 按填写的实网配置调用门面
//
// @author zwei
package llm

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

// liveOpenAIConfig 实网 OpenAI 兼容配置；测前填 APIURL、APIKey、Model
var liveOpenAIConfig = Config{
	Protocol: ProtocolOpenAI,
	APIURL:   "https://api.deepseek.com",
	APIKey:   "",
	Model:    "deepseek-flash",
	Timeout:  60 * time.Second,
}

// liveAnthropicConfig 实网 Anthropic 兼容配置；测前填 APIURL、APIKey、Model
var liveAnthropicConfig = Config{
	Protocol: ProtocolAnthropic,
	APIURL:   "https://api.deepseek.com/anthropic",
	APIKey:   "",
	Model:    "deepseek-flash",
	Timeout:  60 * time.Second,
}

// liveCases 返回两套实网对话配置
func liveCases() []struct {
	name string
	cfg  Config
} {
	return []struct {
		name string
		cfg  Config
	}{
		{"openai", liveOpenAIConfig},
		{"anthropic", liveAnthropicConfig},
	}
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
	for _, tc := range liveCases() {
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

// TestChatStream 分别用 OpenAI 与 Anthropic 两套配置发起流式 Chat
func TestChatStream(t *testing.T) {
	for _, tc := range liveCases() {
		t.Run(tc.name, func(t *testing.T) {
			cli, err := New(requireLiveConfig(t, tc.cfg))
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			stream, err := cli.ChatStream(context.Background(), ChatRequest{
				Messages: []Message{{Role: RoleUser, Content: "ping"}},
			})
			if err != nil {
				t.Fatalf("ChatStream: %v", err)
			}
			defer func() { _ = stream.Close() }()
			var (
				got   strings.Builder
				model string
			)
			for {
				chunk, err := stream.Recv()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("Recv: %v", err)
				}
				if chunk == nil {
					t.Fatalf("chunk=nil")
					return
				}
				if chunk.Model != "" {
					model = chunk.Model
				}
				got.WriteString(chunk.Delta)
			}
			if strings.TrimSpace(got.String()) == "" {
				t.Fatalf("empty stream text")
			}
			t.Logf("protocol=%s model=%s content=%q", tc.cfg.Protocol, model, got.String())
		})
	}
}
