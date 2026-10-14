// Package aiclient Jev 构造与入参校验单测
//
// @author zwei
package aiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

// testJevConfig 千问决策模型（System One）；实网测前填 APIURL、APIKey、Model
var testJevConfig = JevConfig{
	Protocol: JevProtocolSystemOne,
	APIURL:   "https://maas.qianwenaiapi.com/compatible-mode/v1/systemone",
	APIKey:   "",
	Model:    "decision-model-preview",
	Timeout:  60 * time.Second,
}

// requireLiveJevConfig 校验实网配置已填密钥与地址；未填则跳过当前子测试
func requireLiveJevConfig(t *testing.T, cfg JevConfig) JevConfig {
	t.Helper()
	if strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.APIURL) == "" {
		t.Skip("填写 testJevConfig 的 APIURL 与 APIKey 后再跑")
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
		logJevDecideResponse(t, "mock response", resp)
		t.Fatalf("unexpected answers")
	}
	logJevDecideResponse(t, "mock response", resp)
}

// logJevDecideResponse 将完整决策响应以 JSON 写入测试日志（需 -v）
func logJevDecideResponse(t *testing.T, label string, resp *JevDecideResponse) {
	t.Helper()
	if resp == nil {
		t.Logf("%s: null", label)
		return
	}
	b, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		t.Logf("%s: json.MarshalIndent failed: %v\nresp=%+v", label, err, resp)
		return
	}
	t.Logf("%s:\n%s", label, string(b))
}

// TestJevDecide 千问 decision-model-preview 实网决策（与官方 curl 示例同形）
func TestJevDecide(t *testing.T) {
	cli, err := NewJev(requireLiveJevConfig(t, testJevConfig))
	if err != nil {
		t.Fatalf("NewJev: %v", err)
	}
	resp, err := cli.Decide(context.Background(), JevDecideRequest{
		State: json.RawMessage(`{"ticket_id":"T-1001","content":"订单支付后超过 24 小时仍未到账，用户无法继续使用核心服务，要求立即处理。"}`),
		Questions: map[string]JevQuestion{
			"department": {
				Type:         JevQuestionChoice,
				Instructions: "应该由哪个团队处理？",
				Criteria:     json.RawMessage(`{"billing":"支付、退款和账单问题","technical":"产品故障和集成问题"}`),
			},
			"escalate": {
				Type:         JevQuestionNoul,
				Instructions: "是否需要立即通知值班人员？",
			},
			"severity": {
				Type:         JevQuestionScore,
				Instructions: "这个问题有多严重？",
				Criteria:     json.RawMessage(`["轻微问题，不影响功能","部分功能受影响，但存在替代方案","核心功能不可用，没有替代方案","造成严重业务或安全影响"]`),
			},
		},
	})
	if err != nil {
		t.Logf("Decide error (full): %+v", err)
		t.Fatalf("Decide: %v", err)
	}
	logJevDecideResponse(t, "Decide response", resp)
	if len(resp.Answers) != 3 {
		t.Fatalf("expected 3 answers, got %d", len(resp.Answers))
	}
	dept, ok := resp.Answers["department"]
	if !ok || dept.Type != "choice" || dept.Choice == nil {
		t.Fatalf("missing or invalid department answer")
	}
	esc, ok := resp.Answers["escalate"]
	if !ok || esc.Type != "noul" || esc.Noul == nil {
		t.Fatalf("missing or invalid escalate answer")
	}
}
