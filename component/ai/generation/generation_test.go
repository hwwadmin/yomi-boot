// 拼接门面测试。
//
// @author zwei

package generation

import (
	"context"
	"strings"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// TestValidateRequest 覆盖入参契约校验
func TestValidateRequest(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		req  GenerateRequest
	}{
		{"空查询", GenerateRequest{Query: "  "}},
		{"空块文本", GenerateRequest{Query: "q", Contexts: []Context{{Content: " \n "}}}},
		{"合法无上下文", GenerateRequest{Query: "q"}},
		{"合法有上下文", GenerateRequest{Query: "q", Contexts: []Context{{Content: "a", Source: "s"}}}},
	}
	for _, c := range cases {
		err := ValidateRequest(c.req)
		if strings.HasPrefix(c.name, "合法") {
			if err != nil {
				t.Fatalf("%s: 合法请求被拒: %v", c.name, err)
			}
			continue
		}
		if !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("%s: 期望 ErrInvalidArgument，实际 %v", c.name, err)
		}
	}
}

// TestBasicGenerate 覆盖最简拼接模板
func TestBasicGenerate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	g := NewBasic()

	// 无上下文：Prompt 即问题原文
	res, err := g.Generate(ctx, GenerateRequest{Query: "  如何退款  "})
	if err != nil {
		t.Fatal(err)
	}
	if res.Prompt != "如何退款" {
		t.Fatalf("无上下文 Prompt = %q", res.Prompt)
	}

	// 有上下文：按序编号，Source 非空时附来源
	res, err = g.Generate(ctx, GenerateRequest{
		Query: "如何退款",
		Contexts: []Context{
			{Content: " 退款流程为…… ", Source: "manual.md"},
			{Content: "发货时效为……"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "如何退款\n\n参考资料：\n[1] 退款流程为……（来源：manual.md）\n[2] 发货时效为……"
	if res.Prompt != want {
		t.Fatalf("Prompt = %q，期望 %q", res.Prompt, want)
	}
}

// TestBasicInvalidRequest 覆盖占位实现的入参校验透传
func TestBasicInvalidRequest(t *testing.T) {
	t.Parallel()
	g := NewBasic()
	cases := []GenerateRequest{
		{Query: " "},
		{Query: "q", Contexts: []Context{{Content: "a"}, {Content: "  "}}},
	}
	for _, req := range cases {
		if _, err := g.Generate(context.Background(), req); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("期望 ErrInvalidArgument，实际 %v", err)
		}
	}
}

// TestValidateResponse 覆盖拼接输出契约校验
func TestValidateResponse(t *testing.T) {
	t.Parallel()
	if err := ValidateResponse(nil); !errors.Is(err, ErrBadResponse) {
		t.Fatalf("nil 响应: %v", err)
	}
	if err := ValidateResponse(&GenerateResponse{Prompt: " \n "}); !errors.Is(err, ErrBadResponse) {
		t.Fatalf("空提示词: %v", err)
	}
	if err := ValidateResponse(&GenerateResponse{Prompt: "q"}); err != nil {
		t.Fatalf("合法输出被拒: %v", err)
	}
}
