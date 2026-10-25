// 重排门面测试。
//
// @author zwei

package rerank

import (
	"context"
	"fmt"
	"math"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// fakeReranker 返回预设响应或错误，用于断言 Engine 的兜底校验与排序
type fakeReranker struct {
	resp *RerankResponse
	err  error
}

// Rerank 实现 Reranker
func (f *fakeReranker) Rerank(ctx context.Context, req RerankRequest) (*RerankResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

// newTestEngine 创建挂接 fake 实现的测试引擎；reranker 为空时验证默认 PassThrough
func newTestEngine(t *testing.T, reranker Reranker) *Engine {
	t.Helper()
	cfg := Config{Reranker: reranker}
	e, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// TestNewDefaultPassThrough 覆盖空配置默认 PassThrough：保序、全 0 分、TopN 截断
func TestNewDefaultPassThrough(t *testing.T) {
	t.Parallel()
	e := newTestEngine(t, nil)
	result, err := e.Rerank(context.Background(), RerankRequest{
		Query: "如何退款",
		Docs:  []string{"退款流程", "发货时效", "发票开具"},
		TopN:  2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 2 {
		t.Fatalf("TopN 截断失效: %v", result.Results)
	}
	for i, r := range result.Results {
		if r.Index != i || r.Score != 0 {
			t.Fatalf("应保序返回且 Score 恒 0: %+v", result.Results)
		}
	}
	if result.Tokens != 0 {
		t.Fatalf("PassThrough 用量应为 0: %d", result.Tokens)
	}
}

// TestRerankInvalidArgument 覆盖入参校验
func TestRerankInvalidArgument(t *testing.T) {
	t.Parallel()
	tooMany := make([]string, MaxDocs+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("doc-%d", i)
	}
	cases := []struct {
		name string
		req  RerankRequest
	}{
		{"空查询", RerankRequest{Query: "  ", Docs: []string{"a"}}},
		{"空候选", RerankRequest{Query: "q"}},
		{"空候选文本", RerankRequest{Query: "q", Docs: []string{"a", "  "}}},
		{"超上限", RerankRequest{Query: "q", Docs: tooMany}},
		{"负 TopN", RerankRequest{Query: "q", Docs: []string{"a"}, TopN: -1}},
	}
	e := newTestEngine(t, nil)
	for _, c := range cases {
		if _, err := e.Rerank(context.Background(), c.req); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("%s: %v", c.name, err)
		}
	}
}

// TestRerankBadResponse 覆盖实现输出兜底校验
func TestRerankBadResponse(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		resp *RerankResponse
	}{
		{"缺失响应", nil},
		{"长度不等", &RerankResponse{Scores: []float32{0.5}}},
		{"NaN 分数", &RerankResponse{Scores: []float32{float32(math.NaN()), 0.5}}},
		{"Inf 分数", &RerankResponse{Scores: []float32{0.5, float32(math.Inf(1))}}},
	}
	for _, c := range cases {
		e := newTestEngine(t, &fakeReranker{resp: c.resp})
		if _, err := e.Rerank(context.Background(), RerankRequest{Query: "q", Docs: []string{"a", "b"}}); !errors.Is(err, ErrBadResponse) {
			t.Fatalf("%s: %v", c.name, err)
		}
	}
}

// TestRerankOrdering 覆盖降序稳定排序与 TopN 语义
func TestRerankOrdering(t *testing.T) {
	t.Parallel()
	// 分数 0.2 / 0.9 / 0.5 / 0.9；两处 0.9 同分，下标 1 应稳定排在下标 3 前
	e := newTestEngine(t, &fakeReranker{resp: &RerankResponse{Scores: []float32{0.2, 0.9, 0.5, 0.9}, Tokens: 42}})
	result, err := e.Rerank(context.Background(), RerankRequest{Query: "q", Docs: []string{"a", "b", "c", "d"}})
	if err != nil {
		t.Fatal(err)
	}
	wantIndexes := []int{1, 3, 2, 0}
	for i, want := range wantIndexes {
		if result.Results[i].Index != want {
			t.Fatalf("排序结果不符: %+v", result.Results)
		}
	}
	if result.Tokens != 42 {
		t.Fatalf("用量应透传: %d", result.Tokens)
	}

	// TopN 超长钳制为全部
	e = newTestEngine(t, &fakeReranker{resp: &RerankResponse{Scores: []float32{0.2, 0.9}}})
	result, err = e.Rerank(context.Background(), RerankRequest{Query: "q", Docs: []string{"a", "b"}, TopN: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 2 {
		t.Fatalf("TopN 超长应钳制: %v", result.Results)
	}
}

// TestRerankTopNTruncate 覆盖 TopN 截断后不与未截断数据共享底层数组
func TestRerankTopNTruncate(t *testing.T) {
	t.Parallel()
	e := newTestEngine(t, &fakeReranker{resp: &RerankResponse{Scores: []float32{0.1, 0.3, 0.2}}})
	result, err := e.Rerank(context.Background(), RerankRequest{Query: "q", Docs: []string{"a", "b", "c"}, TopN: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 1 || result.Results[0].Index != 1 {
		t.Fatalf("TopN 截断不符: %+v", result.Results)
	}
	result.Results = append(result.Results, RankedDoc{Index: 99})
	if result.Results[1].Index != 99 {
		t.Fatalf("截断结果应可安全追加")
	}
}

// TestRerankTokensClamp 覆盖负用量钳 0
func TestRerankTokensClamp(t *testing.T) {
	t.Parallel()
	e := newTestEngine(t, &fakeReranker{resp: &RerankResponse{Scores: []float32{1}, Tokens: -3}})
	result, err := e.Rerank(context.Background(), RerankRequest{Query: "q", Docs: []string{"a"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Tokens != 0 {
		t.Fatalf("负用量应钳 0: %d", result.Tokens)
	}
}

// TestRerankErrorPassthrough 覆盖实现错误原样透传
func TestRerankErrorPassthrough(t *testing.T) {
	t.Parallel()
	upstream := errors.New("上游超时")
	e := newTestEngine(t, &fakeReranker{err: upstream})
	if _, err := e.Rerank(context.Background(), RerankRequest{Query: "q", Docs: []string{"a"}}); !errors.Is(err, upstream) {
		t.Fatalf("实现错误应透传: %v", err)
	}
}
