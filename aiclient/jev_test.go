// Package aiclient Jev 构造与入参校验单测
//
// @author zwei
package aiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"
)

// validJevCfg 返回可通过构造校验的占位配置；不发起网络
func validJevCfg() JevConfig {
	return JevConfig{
		Protocol: JevProtocolSystemOne,
		APIURL:   "https://opencode.ai/zen/v1/systemone",
		APIKey:   "sk-test",
		Model:    "jev-1.13-free",
	}
}

// TestNewJevRejectsInvalidConfig 验证非法协议、空字段与负超时被拒绝
func TestNewJevRejectsInvalidConfig(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		mut  func(*JevConfig)
	}{
		{"empty protocol", func(c *JevConfig) { c.Protocol = "" }},
		{"unknown protocol", func(c *JevConfig) { c.Protocol = JevProtocol("openai") }},
		{"empty url", func(c *JevConfig) { c.APIURL = "  " }},
		{"empty model", func(c *JevConfig) { c.Model = "\t" }},
		{"negative timeout", func(c *JevConfig) { c.Timeout = -time.Second }},
		{"reserved header", func(c *JevConfig) { c.ExtraHeaders = map[string]string{"Authorization": "x"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validJevCfg()
			tc.mut(&cfg)
			_, err := NewJev(cfg)
			if !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

// TestNewJevCopiesConfig 验证构造后修改原配置不影响已拷贝字段
func TestNewJevCopiesConfig(t *testing.T) {
	t.Parallel()
	cfg := validJevCfg()
	cfg.ExtraHeaders = map[string]string{"X-Gateway": "a"}
	cli, err := NewJev(cfg)
	if err != nil {
		t.Fatalf("NewJev: %v", err)
	}
	impl, ok := cli.(*systemoneJevClient)
	if !ok {
		t.Fatalf("type=%T", cli)
	}
	cfg.ExtraHeaders["X-Gateway"] = "b"
	cfg.Model = "changed"
	if impl.cfg.Model != "jev-1.13-free" {
		t.Fatalf("model=%s", impl.cfg.Model)
	}
	if impl.cfg.ExtraHeaders["X-Gateway"] != "a" {
		t.Fatalf("headers=%v", impl.cfg.ExtraHeaders)
	}
}

// TestNewJevAllowsEmptyAPIKey 验证密钥为空或仅空白时仍可构造
func TestNewJevAllowsEmptyAPIKey(t *testing.T) {
	t.Parallel()
	cfg := validJevCfg()
	cfg.APIKey = "  "
	cli, err := NewJev(cfg)
	if err != nil {
		t.Fatalf("NewJev: %v", err)
	}
	impl, ok := cli.(*systemoneJevClient)
	if !ok {
		t.Fatalf("type=%T", cli)
	}
	if impl.cfg.APIKey != "" {
		t.Fatalf("APIKey=%q", impl.cfg.APIKey)
	}
}

// TestValidateJevDecideRequest 验证空 state、非法 JSON 与空 questions
func TestValidateJevDecideRequest(t *testing.T) {
	t.Parallel()
	valid := JevDecideRequest{
		State: JevStateString("hello"),
		Questions: map[string]JevQuestion{
			"q": {Type: JevQuestionNoul, Instructions: "test?"},
		},
	}
	if err := validateJevDecideRequest(valid); err != nil {
		t.Fatalf("valid: %v", err)
	}
	cases := []JevDecideRequest{
		{State: nil, Questions: valid.Questions},
		{State: json.RawMessage("not-json"), Questions: valid.Questions},
		{State: valid.State, Questions: nil},
		{State: valid.State, Questions: map[string]JevQuestion{"": {Type: JevQuestionNoul, Instructions: "x"}}},
		{State: valid.State, Questions: map[string]JevQuestion{"q": {Type: JevQuestionType("bad"), Instructions: "x"}}},
		{State: valid.State, Questions: map[string]JevQuestion{"q": {Type: JevQuestionNoul, Instructions: "  "}}},
	}
	for i, req := range cases {
		if err := validateJevDecideRequest(req); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("case %d: err=%v", i, err)
		}
	}
}

// TestMustNewJevPanics 验证非法配置 Fail Fast
func TestMustNewJevPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	MustNewJev(JevConfig{})
}

// testJevConfig OpenCode Zen 免费 Jev；实网测前设环境变量 AICLIENT_JEV_API_KEY（可空则走免鉴权路由）
var testJevConfig = JevConfig{
	Protocol: JevProtocolSystemOne,
	APIURL:   "https://opencode.ai/zen/v1/systemone",
	APIKey:   "",
	Model:    "jev-1.13-free",
	Timeout:  60 * time.Second,
}

// requireLiveJevConfig 实网 Jev：须填 APIURL，且设置 AICLIENT_JEV_LIVE=1 或 AICLIENT_JEV_API_KEY
func requireLiveJevConfig(t *testing.T, cfg JevConfig) JevConfig {
	t.Helper()
	if strings.TrimSpace(cfg.APIURL) == "" {
		t.Skip("填写 testJevConfig 的 APIURL 后再跑")
	}
	key := strings.TrimSpace(os.Getenv("AICLIENT_JEV_API_KEY"))
	live := strings.TrimSpace(os.Getenv("AICLIENT_JEV_LIVE")) == "1"
	if !live && key == "" {
		t.Skip("设置 AICLIENT_JEV_LIVE=1（免鉴权试跑）或 AICLIENT_JEV_API_KEY 后再跑实网")
	}
	if key != "" {
		cfg.APIKey = key
	}
	return cfg
}

// TestJevDecideMock 验证 System One JSON 请求与响应解析
func TestJevDecideMock(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		var body systemoneDecideRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if body.Model != "jev-mock" || len(body.Questions) != 1 {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "jev-mock",
			"answers": map[string]any{
				"is_urgent": map[string]any{"type": "noul", "noul": 0.9},
			},
			"usage": map[string]any{"input_tokens": 10, "output_tokens": 2},
		})
	}))
	defer srv.Close()
	cli, err := NewJev(JevConfig{
		Protocol: JevProtocolSystemOne,
		APIURL:   srv.URL,
		Model:    "jev-mock",
	})
	if err != nil {
		t.Fatalf("NewJev: %v", err)
	}
	resp, err := cli.Decide(context.Background(), JevDecideRequest{
		State: JevStateString("test"),
		Questions: map[string]JevQuestion{
			"is_urgent": {Type: JevQuestionNoul, Instructions: "urgent?"},
		},
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if resp.Answers["is_urgent"].Noul == nil || *resp.Answers["is_urgent"].Noul != 0.9 {
		t.Fatalf("resp=%+v", resp)
	}
}

// TestJevDecide 用 OpenCode systemone 发起同步决策
func TestJevDecide(t *testing.T) {
	cli, err := NewJev(requireLiveJevConfig(t, testJevConfig))
	if err != nil {
		t.Fatalf("NewJev: %v", err)
	}
	resp, err := cli.Decide(context.Background(), JevDecideRequest{
		State: JevStateString("付款已经失败三天，请尽快处理。"),
		Questions: map[string]JevQuestion{
			"is_urgent": {
				Type:         JevQuestionNoul,
				Instructions: "是否需要紧急处理？",
			},
		},
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	ans, ok := resp.Answers["is_urgent"]
	if !ok || ans.Type != "noul" || ans.Noul == nil {
		t.Fatalf("resp=%+v", resp)
	}
	t.Logf("model=%s noul=%v usage=%+v", resp.Model, *ans.Noul, resp.Usage)
}
