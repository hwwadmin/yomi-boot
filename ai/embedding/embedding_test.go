// 向量化构造与入参校验单测
//
// @author zwei

package embedding

import (
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"

	openai "github.com/openai/openai-go/v3"
)

// validCfg 返回可通过构造校验的占位配置；不发起网络
func validCfg() Config {
	return Config{
		Protocol: ProtocolOpenAI,
		APIURL:   "https://api.openai.com/v1",
		APIKey:   "sk-test",
		Model:    "text-embedding-3-small",
	}
}

// TestNewRejectsInvalidConfig 验证非法协议、空字段、非正维度与负超时被拒绝
func TestNewRejectsInvalidConfig(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		mut  func(*Config)
	}{
		{"empty protocol", func(c *Config) { c.Protocol = "" }},
		{"unknown protocol", func(c *Config) { c.Protocol = Protocol("anthropic") }},
		{"empty url", func(c *Config) { c.APIURL = "  " }},
		{"bad url", func(c *Config) { c.APIURL = "not-a-url" }},
		{"bad scheme", func(c *Config) { c.APIURL = "ftp://example.com" }},
		{"empty model", func(c *Config) { c.Model = "\t" }},
		{"non-positive dimensions", func(c *Config) { z := 0; c.Dimensions = &z }},
		{"negative max batch", func(c *Config) { c.MaxBatch = -1 }},
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
	dim := 8
	cfg := validCfg()
	cfg.Dimensions = &dim
	cfg.ExtraHeaders = map[string]string{"X-Gateway": "a"}
	cli, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	impl, ok := cli.(*openaiClient)
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

// TestNewAllowsEmptyAPIKey 验证密钥为空或仅空白时仍可构造
func TestNewAllowsEmptyAPIKey(t *testing.T) {
	t.Parallel()
	cfg := validCfg()
	cfg.APIKey = "  "
	cli, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	impl, ok := cli.(*openaiClient)
	if !ok {
		t.Fatalf("type=%T", cli)
	}
	if impl.cfg.APIKey != "" {
		t.Fatalf("APIKey=%q", impl.cfg.APIKey)
	}
}

// TestValidateRequest 验证空列表、trim 后空串与非正维度
func TestValidateRequest(t *testing.T) {
	t.Parallel()
	zero := 0
	cases := []Request{
		{},
		{Inputs: []string{"ok", "  "}},
		{Inputs: []string{"ok"}, Dimensions: &zero},
	}
	for i, req := range cases {
		if err := validateRequest(req); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("case %d err=%v", i, err)
		}
	}
	if err := validateRequest(Request{Inputs: []string{" a "}}); err != nil {
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
	if u == nil || u.PromptTokens != 3 || u.TotalTokens != 3 {
		t.Fatalf("usage=%+v", u)
	}
}

// TestMustNewPanics 验证非法配置 Fail Fast
func TestMustNewPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	MustNew(Config{})
}

// TestEmbedRejectsEmptyInputs 验证构造成功后空输入仍走本包校验
func TestEmbedRejectsEmptyInputs(t *testing.T) {
	t.Parallel()
	cli, err := New(validCfg())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = cli.Embed(t.Context(), Request{})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("err=%v", err)
	}
}
