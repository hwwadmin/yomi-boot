// Package jev Jev 构造与入参校验单测
//
// @author zwei
package jev

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"
)

// validCfg 返回可通过构造校验的占位配置；不发起网络
func validCfg() Config {
	return Config{
		Protocol: ProtocolSystemOne,
		APIURL:   "https://opencode.ai/zen/v1/systemone",
		APIKey:   "sk-test",
		Model:    "jev-1.13-free",
	}
}

// TestNewRejectsInvalidConfig 验证非法协议、空字段与负超时被拒绝
func TestNewRejectsInvalidConfig(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		mut  func(*Config)
	}{
		{"empty protocol", func(c *Config) { c.Protocol = "" }},
		{"unknown protocol", func(c *Config) { c.Protocol = Protocol("openai") }},
		{"empty url", func(c *Config) { c.APIURL = "  " }},
		{"bad url", func(c *Config) { c.APIURL = "not-a-url" }},
		{"bad scheme", func(c *Config) { c.APIURL = "ftp://example.com" }},
		{"empty model", func(c *Config) { c.Model = "\t" }},
		{"negative timeout", func(c *Config) { c.Timeout = -time.Second }},
		{"reserved header", func(c *Config) { c.ExtraHeaders = map[string]string{"Authorization": "x"} }},
		{"blank header key", func(c *Config) { c.ExtraHeaders = map[string]string{" ": "x"} }},
		{"duplicate trimmed key", func(c *Config) { c.ExtraHeaders = map[string]string{"X-A": "1", " X-A ": "2"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validCfg()
			tc.mut(&cfg)
			_, err := New(cfg)
			if !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

// TestNewCopiesConfig 验证构造后修改原配置不影响已拷贝字段
func TestNewCopiesConfig(t *testing.T) {
	t.Parallel()
	cfg := validCfg()
	cfg.ExtraHeaders = map[string]string{"X-Gateway": "a"}
	cli, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	impl, ok := cli.(*systemOneClient)
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

// TestNewAllowsEmptyAPIKey 验证密钥为空或仅空白时仍可构造
func TestNewAllowsEmptyAPIKey(t *testing.T) {
	t.Parallel()
	cfg := validCfg()
	cfg.APIKey = "  "
	cli, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	impl, ok := cli.(*systemOneClient)
	if !ok {
		t.Fatalf("type=%T", cli)
	}
	if impl.cfg.APIKey != "" {
		t.Fatalf("APIKey=%q", impl.cfg.APIKey)
	}
}

// TestValidateDecideRequest 验证空 state、非法 JSON 与空 questions
func TestValidateDecideRequest(t *testing.T) {
	t.Parallel()
	valid := DecideRequest{
		State: StateString("hello"),
		Questions: map[string]Question{
			"q": {Type: QuestionNoul, Instructions: "test?"},
		},
	}
	if err := validateDecideRequest(valid); err != nil {
		t.Fatalf("valid: %v", err)
	}
	cases := []DecideRequest{
		{State: nil, Questions: valid.Questions},
		{State: json.RawMessage("not-json"), Questions: valid.Questions},
		{State: json.RawMessage("null"), Questions: valid.Questions},
		{State: valid.State, Questions: nil},
		{State: valid.State, Questions: map[string]Question{"": {Type: QuestionNoul, Instructions: "x"}}},
		{State: valid.State, Questions: map[string]Question{"q": {Type: QuestionType("bad"), Instructions: "x"}}},
		{State: valid.State, Questions: map[string]Question{"q": {Type: QuestionNoul, Instructions: "  "}}},
		{State: valid.State, Questions: map[string]Question{"q": {Type: QuestionNoul, Instructions: "x", Criteria: json.RawMessage(`{"a":1}`)}}},
		{State: valid.State, Questions: map[string]Question{"q": {Type: QuestionChoice, Instructions: "x", Criteria: json.RawMessage("not-json")}}},
		{State: valid.State, Questions: map[string]Question{"q": {Type: QuestionChoice, Instructions: "x", Criteria: json.RawMessage(`["a"]`)}}},
		{State: valid.State, Questions: map[string]Question{"q": {Type: QuestionScore, Instructions: "x", Criteria: json.RawMessage(`{"a":1}`)}}},
	}
	for i, req := range cases {
		if err := validateDecideRequest(req); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("case %d: err=%v", i, err)
		}
	}
	validCriteria := DecideRequest{
		State: valid.State,
		Questions: map[string]Question{
			"choice": {Type: QuestionChoice, Instructions: "x", Criteria: json.RawMessage(`{"a":"b"}`)},
			"score":  {Type: QuestionScore, Instructions: "x", Criteria: json.RawMessage(`["a","b"]`)},
		},
	}
	if err := validateDecideRequest(validCriteria); err != nil {
		t.Fatalf("valid criteria: %v", err)
	}
}

// TestDecideRejectsMissingOrMismatchedAnswers 验证缺答与答案类型不符返回 ErrBadResponse
func TestDecideRejectsMissingOrMismatchedAnswers(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		answers map[string]any
	}{
		{"missing", map[string]any{}},
		{"type mismatch", map[string]any{"is_urgent": map[string]any{"type": "choice", "choice": "a"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"model":   "jev-mock",
					"answers": tc.answers,
				})
			}))
			defer srv.Close()
			cli, err := New(Config{Protocol: ProtocolSystemOne, APIURL: srv.URL, Model: "jev-mock"})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			_, err = cli.Decide(context.Background(), DecideRequest{
				State: StateString("test"),
				Questions: map[string]Question{
					"is_urgent": {Type: QuestionNoul, Instructions: "urgent?"},
				},
			})
			if !errors.Is(err, ErrBadResponse) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

// TestMustNewPanics 验证非法配置 Fail Fast
func TestMustNewPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	MustNew(Config{})
}

// TestDecideMock 验证 System One JSON 请求与响应解析
func TestDecideMock(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		var body systemOneDecideRequest
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
	cli, err := New(Config{
		Protocol: ProtocolSystemOne,
		APIURL:   srv.URL,
		Model:    "jev-mock",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	resp, err := cli.Decide(context.Background(), DecideRequest{
		State: StateString("test"),
		Questions: map[string]Question{
			"is_urgent": {Type: QuestionNoul, Instructions: "urgent?"},
		},
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if resp.Answers["is_urgent"].Noul == nil || *resp.Answers["is_urgent"].Noul != 0.9 {
		logDecideResponse(t, "mock response", resp)
		t.Fatalf("unexpected answers")
	}
	logDecideResponse(t, "mock response", resp)
}

// logDecideResponse 将完整决策响应以 JSON 写入测试日志（需 -v）
func logDecideResponse(t *testing.T, label string, resp *DecideResponse) {
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
