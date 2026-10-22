// 配置与 max token 字段单测
//
// @author zwei

package llm

import (
	"testing"

	"gitee.com/hq_project/yomi-boot/core/errors"
)

// TestResolveMaxTokensField 验证自动模式按模型族选择字段，显式配置优先
func TestResolveMaxTokensField(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cfg  Config
		want MaxTokensField
	}{
		{"gpt-4o auto", Config{Model: "gpt-4o"}, MaxTokensFieldLegacy},
		{"deepseek auto", Config{Model: "deepseek-flash"}, MaxTokensFieldLegacy},
		{"o1 auto", Config{Model: "o1-mini"}, MaxTokensFieldCompletion},
		{"o3 auto", Config{Model: "o3"}, MaxTokensFieldCompletion},
		{"o4 auto", Config{Model: "o4-mini"}, MaxTokensFieldCompletion},
		{"gpt-5 auto", Config{Model: "gpt-5"}, MaxTokensFieldCompletion},
		{"provider prefix auto", Config{Model: "openai/o1-mini"}, MaxTokensFieldCompletion},
		{"explicit legacy", Config{Model: "o1", MaxTokensField: MaxTokensFieldLegacy}, MaxTokensFieldLegacy},
		{"explicit completion", Config{Model: "gpt-4o", MaxTokensField: MaxTokensFieldCompletion}, MaxTokensFieldCompletion},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.resolvedMaxTokensField(); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestBuildParamsUsesSingleMaxTokensField 验证请求体只写一个 max token 字段
func TestBuildParamsUsesSingleMaxTokensField(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		cfg          Config
		wantLegacy   bool
		wantComplete bool
	}{
		{"gpt-4o", Config{Model: "gpt-4o"}, true, false},
		{"o1-mini", Config{Model: "o1-mini"}, false, true},
		{"explicit completion", Config{Model: "gateway-alias", MaxTokensField: MaxTokensFieldCompletion}, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.cfg
			cfg.Protocol = ProtocolOpenAI
			cfg.APIURL = "https://api.openai.com/v1"
			cfg.APIKey = "sk-test"
			cli, err := New(cfg)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			impl, ok := cli.(*openaiClient)
			if !ok {
				t.Fatalf("type=%T", cli)
			}
			params, err := impl.buildParams(ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
			if err != nil {
				t.Fatalf("buildParams: %v", err)
			}
			if params.MaxTokens.Valid() != tc.wantLegacy {
				t.Fatalf("MaxTokens valid=%v", params.MaxTokens.Valid())
			}
			if params.MaxCompletionTokens.Valid() != tc.wantComplete {
				t.Fatalf("MaxCompletionTokens valid=%v", params.MaxCompletionTokens.Valid())
			}
		})
	}
}

// TestAnthropicRejectsBlankContent 验证 Anthropic 非 system 空白内容在发请求前被拒绝
func TestAnthropicRejectsBlankContent(t *testing.T) {
	t.Parallel()
	cli, err := New(Config{
		Protocol: ProtocolAnthropic,
		APIURL:   "https://api.anthropic.com",
		APIKey:   "sk-test",
		Model:    "claude-3-5-sonnet",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	impl, ok := cli.(*anthropicClient)
	if !ok {
		t.Fatalf("type=%T", cli)
	}
	if _, err := impl.buildParams(ChatRequest{Messages: []Message{{Role: RoleUser, Content: "  "}}}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("err=%v", err)
	}
}

// TestNewRejectsInvalidURLHeaderAndField 验证非法地址、扩展头与 max token 字段被拒绝
func TestNewRejectsInvalidURLHeaderAndField(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		mut  func(*Config)
	}{
		{"bad url", func(c *Config) { c.APIURL = "not-a-url" }},
		{"bad scheme", func(c *Config) { c.APIURL = "ftp://example.com" }},
		{"blank header key", func(c *Config) { c.ExtraHeaders = map[string]string{" ": "x"} }},
		{"duplicate trimmed key", func(c *Config) { c.ExtraHeaders = map[string]string{"X-A": "1", " X-A ": "2"} }},
		{"bad max tokens field", func(c *Config) { c.MaxTokensField = MaxTokensField("both") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{
				Protocol: ProtocolOpenAI,
				APIURL:   "https://api.openai.com/v1",
				APIKey:   "sk-test",
				Model:    "gpt-4o",
			}
			tc.mut(&cfg)
			if _, err := New(cfg); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}
