// 配置校验、门面辅助与错误映射单测。
//
// @author zwei

package llm

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// TestValidateChatRequest 覆盖消息列表、角色与最大令牌数校验
func TestValidateChatRequest(t *testing.T) {
	t.Parallel()
	zero, neg := 0, -1
	cases := []struct {
		name string
		req  ChatRequest
		ok   bool
	}{
		{"empty messages", ChatRequest{}, false},
		{"unknown role", ChatRequest{Messages: []Message{{Role: Role("tool"), Content: "x"}}}, false},
		{"zero max tokens", ChatRequest{Messages: []Message{{Role: RoleUser, Content: "x"}}, MaxTokens: &zero}, false},
		{"negative max tokens", ChatRequest{Messages: []Message{{Role: RoleUser, Content: "x"}}, MaxTokens: &neg}, false},
		{"system only", ChatRequest{Messages: []Message{{Role: RoleSystem, Content: "x"}}}, true},
		{"assistant only", ChatRequest{Messages: []Message{{Role: RoleAssistant, Content: "x"}}}, true},
		{"valid", ChatRequest{Messages: []Message{{Role: RoleUser, Content: "x"}}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateChatRequest(tc.req)
			if tc.ok && err != nil {
				t.Fatalf("err=%v", err)
			}
			if !tc.ok && !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

// TestConfigValidateRemaining 覆盖类型与数值类非法配置
func TestConfigValidateRemaining(t *testing.T) {
	t.Parallel()
	negMax := -1
	cases := []struct {
		name string
		mut  func(*Config)
	}{
		{"bad protocol", func(c *Config) { c.Protocol = Protocol("gemini") }},
		{"blank api key", func(c *Config) { c.APIKey = " " }},
		{"blank model", func(c *Config) { c.Model = " " }},
		{"negative max tokens", func(c *Config) { c.MaxTokens = &negMax }},
		{"negative timeout", func(c *Config) { c.Timeout = -time.Second }},
		{"reserved authorization", func(c *Config) { c.ExtraHeaders = map[string]string{"Authorization": "x"} }},
		{"reserved x-api-key", func(c *Config) { c.ExtraHeaders = map[string]string{"X-API-Key": "x"} }},
		{"reserved anthropic version", func(c *Config) { c.ExtraHeaders = map[string]string{"anthropic-version": "x"} }},
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
			if err := cfg.validate(); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

// TestConfigHelpers 覆盖默认值、保留头与克隆辅助
func TestConfigHelpers(t *testing.T) {
	t.Parallel()
	if got := timeoutOrDefault(0); got != defaultTimeout {
		t.Fatalf("timeoutOrDefault(0)=%v", got)
	}
	if got := timeoutOrDefault(3 * time.Second); got != 3*time.Second {
		t.Fatalf("timeoutOrDefault=%v", got)
	}
	if got := (Config{}).anthropicVersionOrDefault(); got != defaultAnthropicVersion {
		t.Fatalf("version=%q", got)
	}
	if got := (Config{AnthropicVersion: " 2024-01-01 "}).anthropicVersionOrDefault(); got != "2024-01-01" {
		t.Fatalf("version=%q", got)
	}
	reserved := []string{"authorization", " X-API-Key ", "anthropic-version", "ANTHROPIC-VERSION"}
	for _, key := range reserved {
		if !isReservedHeader(key) {
			t.Fatalf("%q should be reserved", key)
		}
	}
	for _, key := range []string{"x-gw", "x-api-key-extra", ""} {
		if isReservedHeader(key) {
			t.Fatalf("%q should not be reserved", key)
		}
	}

	if got := copyExtraHeaders(nil); got != nil {
		t.Fatalf("copy nil=%v", got)
	}
	src := map[string]string{" X-A ": "1"}
	dst := copyExtraHeaders(src)
	if dst["X-A"] != "1" || len(dst) != 1 {
		t.Fatalf("copy=%v", dst)
	}
	dst["X-A"] = "2"
	if src[" X-A "] != "1" {
		t.Fatal("copy shares source map")
	}

	if cloneFloat64(nil) != nil {
		t.Fatal("clone nil float")
	}
	f := 0.5
	cf := cloneFloat64(&f)
	f = 0.9
	if *cf != 0.5 {
		t.Fatalf("clone float=%v", *cf)
	}
	if cloneInt(nil) != nil {
		t.Fatal("clone nil int")
	}
	n := 3
	cn := cloneInt(&n)
	n = 4
	if *cn != 3 {
		t.Fatalf("clone int=%v", *cn)
	}
}

// TestApplyGenerationDefaults 验证空采样参数填入缺省
func TestApplyGenerationDefaults(t *testing.T) {
	t.Parallel()
	cfg := Config{}
	applyGenerationDefaults(&cfg)
	if cfg.Temperature == nil || *cfg.Temperature != defaultTemperature {
		t.Fatalf("temperature=%v", cfg.Temperature)
	}
	if cfg.MaxTokens == nil || *cfg.MaxTokens != defaultMaxTokens {
		t.Fatalf("max tokens=%v", cfg.MaxTokens)
	}
	f := 0.0
	n := 1
	cfg = Config{Temperature: &f, MaxTokens: &n}
	applyGenerationDefaults(&cfg)
	if *cfg.Temperature != 0 || *cfg.MaxTokens != 1 {
		t.Fatalf("explicit values overwritten: %v %v", *cfg.Temperature, *cfg.MaxTokens)
	}
}

// TestNewCopiesAndTrimsConfig 验证构造期拷贝配置且归一化字段
func TestNewCopiesAndTrimsConfig(t *testing.T) {
	t.Parallel()
	temp := 0.4
	maxTokens := 10
	headers := map[string]string{"X-A": "1"}
	cli, err := New(Config{
		Protocol:     ProtocolOpenAI,
		APIURL:       " https://api.openai.com/v1 ",
		APIKey:       " sk-test ",
		Model:        " gpt-4o ",
		Temperature:  &temp,
		MaxTokens:    &maxTokens,
		ExtraHeaders: headers,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	impl, ok := cli.(*openaiClient)
	if !ok {
		t.Fatalf("type=%T", cli)
	}
	temp = 0.9
	maxTokens = 99
	headers["X-A"] = "2"
	if impl.cfg.APIURL != "https://api.openai.com/v1" || impl.cfg.APIKey != "sk-test" || impl.cfg.Model != "gpt-4o" {
		t.Fatalf("cfg=%+v", impl.cfg)
	}
	if impl.cfg.Temperature == nil || *impl.cfg.Temperature != 0.4 {
		t.Fatalf("temperature=%v", impl.cfg.Temperature)
	}
	if impl.cfg.MaxTokens == nil || *impl.cfg.MaxTokens != 10 {
		t.Fatalf("max tokens=%v", impl.cfg.MaxTokens)
	}
	if impl.cfg.ExtraHeaders["X-A"] != "1" {
		t.Fatalf("headers=%v", impl.cfg.ExtraHeaders)
	}
}

// TestMustNew 验证成功返回客户端、失败 panic
func TestMustNew(t *testing.T) {
	t.Parallel()
	cli := MustNew(Config{
		Protocol: ProtocolOpenAI,
		APIURL:   "https://api.openai.com/v1",
		APIKey:   "sk-test",
		Model:    "gpt-4o",
	})
	if cli == nil {
		t.Fatal("MustNew returned nil")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("MustNew should panic on invalid config")
		}
	}()
	MustNew(Config{Protocol: ProtocolOpenAI})
}

// TestMapStatusError 覆盖状态码到哨兵的映射与空原因
func TestMapStatusError(t *testing.T) {
	t.Parallel()
	cause := errors.New("upstream boom")
	if err := mapStatusError(0, nil); err != nil {
		t.Fatalf("nil cause=%v", err)
	}
	cases := []struct {
		name   string
		status int
		want   error
	}{
		{"unauthorized", http.StatusUnauthorized, ErrUnauthorized},
		{"forbidden", http.StatusForbidden, ErrUnauthorized},
		{"rate limited", http.StatusTooManyRequests, ErrRateLimited},
		{"bad request", http.StatusBadRequest, ErrBadResponse},
		{"server error", http.StatusInternalServerError, ErrBadResponse},
		{"unknown status", 0, ErrBadResponse},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := mapStatusError(tc.status, cause)
			if !errors.Is(err, tc.want) || !errors.Is(err, cause) {
				t.Fatalf("err=%v", err)
			}
		})
	}
	if err := mapStatusError(500, context.DeadlineExceeded); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ctx cause=%v", err)
	}
}

// TestWrapSentinelWithoutCause 验证空原因只保留哨兵栈
func TestWrapSentinelWithoutCause(t *testing.T) {
	t.Parallel()
	err := wrapSentinel(ErrBadResponse, nil)
	if !errors.Is(err, ErrBadResponse) {
		t.Fatalf("err=%v", err)
	}
}

// TestWrapCtxErrPassThrough 验证非上下文错误原样返回
func TestWrapCtxErrPassThrough(t *testing.T) {
	t.Parallel()
	if err := wrapCtxErr(nil); err != nil {
		t.Fatalf("nil=%v", err)
	}
	if got := wrapCtxErr(io.EOF); got != io.EOF {
		t.Fatalf("got %v", got)
	}
}

// TestResolveGenerationParams 覆盖请求参数与配置缺省的优先级
func TestResolveGenerationParams(t *testing.T) {
	t.Parallel()
	if resolveTemperature(nil, nil) != nil || resolveMaxTokens(nil, nil) != nil {
		t.Fatal("nil defaults should stay nil")
	}
	defTemp, defMax := 0.1, 2
	reqTemp, reqMax := 0.2, 3
	if got := resolveTemperature(nil, &defTemp); got != &defTemp {
		t.Fatalf("def temp=%v", got)
	}
	if got := resolveTemperature(&reqTemp, &defTemp); got != &reqTemp {
		t.Fatalf("req temp=%v", got)
	}
	if got := resolveMaxTokens(nil, &defMax); got != &defMax {
		t.Fatalf("def max=%v", got)
	}
	if got := resolveMaxTokens(&reqMax, &defMax); got != &reqMax {
		t.Fatalf("req max=%v", got)
	}
}
