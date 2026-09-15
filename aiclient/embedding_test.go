// Package aiclient 向量化构造与入参校验单测
//
// @author zwei
package aiclient

import (
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"

	openai "github.com/openai/openai-go/v3"
)

// validEmbeddingCfg 返回可通过构造校验的占位配置；不发起网络
func validEmbeddingCfg() EmbeddingConfig {
	return EmbeddingConfig{
		Protocol: EmbeddingProtocolOpenAI,
		APIURL:   "https://api.openai.com/v1",
		APIKey:   "sk-test",
		Model:    "text-embedding-3-small",
	}
}

// TestNewEmbeddingRejectsInvalidConfig 验证非法协议、空字段、非正维度与负超时被拒绝
func TestNewEmbeddingRejectsInvalidConfig(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		mut  func(*EmbeddingConfig)
	}{
		{"empty protocol", func(c *EmbeddingConfig) { c.Protocol = "" }},
		{"unknown protocol", func(c *EmbeddingConfig) { c.Protocol = EmbeddingProtocol("anthropic") }},
		{"empty url", func(c *EmbeddingConfig) { c.APIURL = "  " }},
		{"empty model", func(c *EmbeddingConfig) { c.Model = "\t" }},
		{"non-positive dimensions", func(c *EmbeddingConfig) { z := 0; c.Dimensions = &z }},
		{"negative timeout", func(c *EmbeddingConfig) { c.Timeout = -time.Second }},
		{"reserved header", func(c *EmbeddingConfig) { c.ExtraHeaders = map[string]string{"Authorization": "x"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validEmbeddingCfg()
			tc.mut(&cfg)
			_, err := NewEmbedding(cfg)
			if !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

// TestNewEmbeddingCopiesConfig 验证构造后修改原配置不影响已拷贝字段
func TestNewEmbeddingCopiesConfig(t *testing.T) {
	t.Parallel()
	dim := 8
	cfg := validEmbeddingCfg()
	cfg.Dimensions = &dim
	cfg.ExtraHeaders = map[string]string{"X-Gateway": "a"}
	cli, err := NewEmbedding(cfg)
	if err != nil {
		t.Fatalf("NewEmbedding: %v", err)
	}
	impl, ok := cli.(*openaiEmbeddingClient)
	if !ok {
		t.Fatalf("type=%T", cli)
	}
	dim = 16
	cfg.ExtraHeaders["X-Gateway"] = "b"
	cfg.Model = "changed"
	if impl.cfg.Model != "text-embedding-3-small" {
		t.Fatalf("model=%s", impl.cfg.Model)
	}
	if impl.cfg.Dimensions == nil || *impl.cfg.Dimensions != 8 {
		t.Fatalf("dimensions=%v", impl.cfg.Dimensions)
	}
	if impl.cfg.ExtraHeaders["X-Gateway"] != "a" {
		t.Fatalf("headers=%v", impl.cfg.ExtraHeaders)
	}
}

// TestNewEmbeddingAllowsEmptyAPIKey 验证密钥为空或仅空白时仍可构造
func TestNewEmbeddingAllowsEmptyAPIKey(t *testing.T) {
	t.Parallel()
	cfg := validEmbeddingCfg()
	cfg.APIKey = "  "
	cli, err := NewEmbedding(cfg)
	if err != nil {
		t.Fatalf("NewEmbedding: %v", err)
	}
	impl, ok := cli.(*openaiEmbeddingClient)
	if !ok {
		t.Fatalf("type=%T", cli)
	}
	if impl.cfg.APIKey != "" {
		t.Fatalf("APIKey=%q", impl.cfg.APIKey)
	}
}

// TestValidateEmbeddingRequest 验证空列表、trim 后空串与非正维度
func TestValidateEmbeddingRequest(t *testing.T) {
	t.Parallel()
	zero := 0
	cases := []EmbeddingRequest{
		{},
		{Inputs: []string{"ok", "  "}},
		{Inputs: []string{"ok"}, Dimensions: &zero},
	}
	for i, req := range cases {
		if err := validateEmbeddingRequest(req); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("case %d err=%v", i, err)
		}
	}
	if err := validateEmbeddingRequest(EmbeddingRequest{Inputs: []string{" a "}}); err != nil {
		t.Fatalf("valid: %v", err)
	}
}

// TestResolveDimensions 验证请求覆盖配置、皆空则不传
func TestResolveDimensions(t *testing.T) {
	t.Parallel()
	req, def := 3, 8
	if got := resolveDimensions(&req, &def); got == nil || *got != 3 {
		t.Fatalf("override=%v", got)
	}
	if got := resolveDimensions(nil, &def); got == nil || *got != 8 {
		t.Fatalf("fallback=%v", got)
	}
	if got := resolveDimensions(nil, nil); got != nil {
		t.Fatalf("both nil=%v", got)
	}
}

// TestCollectEmbeddingVectors 验证按下标对齐、缺条、空向量与重复下标
func TestCollectEmbeddingVectors(t *testing.T) {
	t.Parallel()
	ok, err := collectEmbeddingVectors(2, []openai.Embedding{
		{Index: 1, Embedding: []float64{0.2}},
		{Index: 0, Embedding: []float64{0.1}},
	})
	if err != nil || len(ok) != 2 || ok[0][0] != 0.1 || ok[1][0] != 0.2 {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	bads := [][]openai.Embedding{
		nil,
		{{Index: 0, Embedding: []float64{1}}},
		{{Index: 0, Embedding: nil}, {Index: 1, Embedding: []float64{1}}},
		{{Index: 0, Embedding: []float64{1}}, {Index: 0, Embedding: []float64{2}}},
		{{Index: -1, Embedding: []float64{1}}, {Index: 0, Embedding: []float64{2}}},
	}
	for i, data := range bads {
		if _, err := collectEmbeddingVectors(2, data); !errors.Is(err, ErrBadResponse) {
			t.Fatalf("case %d err=%v", i, err)
		}
	}
}

// TestEmbeddingUsage 验证全 0 为空且不填 completion
func TestEmbeddingUsage(t *testing.T) {
	t.Parallel()
	if embeddingUsage(0, 0) != nil {
		t.Fatal("zero should be nil")
	}
	u := embeddingUsage(3, 3)
	if u == nil || u.PromptTokens != 3 || u.TotalTokens != 3 || u.CompletionTokens != 0 {
		t.Fatalf("usage=%+v", u)
	}
}

// TestMustNewEmbeddingPanics 验证非法配置 Fail Fast
func TestMustNewEmbeddingPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	MustNewEmbedding(EmbeddingConfig{})
}

// TestEmbedLocalValidate 验证构造成功后空输入仍走本包校验
func TestEmbedLocalValidate(t *testing.T) {
	t.Parallel()
	cli, err := NewEmbedding(validEmbeddingCfg())
	if err != nil {
		t.Fatalf("NewEmbedding: %v", err)
	}
	_, err = cli.Embed(t.Context(), EmbeddingRequest{})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("err=%v", err)
	}
}
