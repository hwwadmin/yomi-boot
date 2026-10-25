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

// TestValidateRequest 覆盖入参契约校验
func TestValidateRequest(t *testing.T) {
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
		{"合法", RerankRequest{Query: "q", Docs: []string{"a"}, TopN: 1}},
	}
	for _, c := range cases {
		err := ValidateRequest(c.req)
		if c.name == "合法" {
			if err != nil {
				t.Fatalf("合法请求被拒: %v", err)
			}
			continue
		}
		if !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("%s: %v", c.name, err)
		}
	}
}

// TestRankScores 覆盖降序稳定排序与 TopN 语义
func TestRankScores(t *testing.T) {
	t.Parallel()
	// 分数 0.2 / 0.9 / 0.5 / 0.9；两处 0.9 同分，下标 1 应稳定排在下标 3 前
	ranked, err := RankScores([]float32{0.2, 0.9, 0.5, 0.9}, 0)
	if err != nil {
		t.Fatal(err)
	}
	wantIndexes := []int{1, 3, 2, 0}
	for i, want := range wantIndexes {
		if ranked[i].Index != want {
			t.Fatalf("排序结果不符: %+v", ranked)
		}
	}

	// TopN 超长钳制为全部
	ranked, err = RankScores([]float32{0.2, 0.9}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(ranked) != 2 {
		t.Fatalf("TopN 超长应钳制: %+v", ranked)
	}

	// 截断后追加安全，不与未截断数据共享底层数组
	ranked, err = RankScores([]float32{0.1, 0.3, 0.2}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(ranked) != 1 || ranked[0].Index != 1 {
		t.Fatalf("TopN 截断不符: %+v", ranked)
	}
	ranked = append(ranked, RankedDoc{Index: 99})
	if ranked[1].Index != 99 {
		t.Fatal("截断结果应可安全追加")
	}

	// 空分数进空出
	ranked, err = RankScores(nil, 0)
	if err != nil || len(ranked) != 0 {
		t.Fatalf("空分数应空出: %v, %v", ranked, err)
	}
}

// TestRankScoresInvalid 覆盖非法分数与负 TopN
func TestRankScoresInvalid(t *testing.T) {
	t.Parallel()
	if _, err := RankScores([]float32{float32(math.NaN()), 0.5}, 0); !errors.Is(err, ErrBadResponse) {
		t.Fatalf("NaN 分数: %v", err)
	}
	if _, err := RankScores([]float32{0.5, float32(math.Inf(-1))}, 0); !errors.Is(err, ErrBadResponse) {
		t.Fatalf("Inf 分数: %v", err)
	}
	if _, err := RankScores([]float32{0.5}, -1); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("负 TopN: %v", err)
	}
}

// TestPassThrough 覆盖空实现：拒非法入参、保序、全 0 分、TopN 截断
func TestPassThrough(t *testing.T) {
	t.Parallel()
	r := NewPassThrough()
	if _, err := r.Rerank(context.Background(), RerankRequest{Query: " ", Docs: []string{"a"}}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("空查询应被拒: %v", err)
	}
	result, err := r.Rerank(context.Background(), RerankRequest{
		Query: "如何退款",
		Docs:  []string{"退款流程", "发货时效", "发票开具"},
		TopN:  2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 2 {
		t.Fatalf("TopN 截断失效: %+v", result.Results)
	}
	for i, doc := range result.Results {
		if doc.Index != i || doc.Score != 0 {
			t.Fatalf("应保序返回且 Score 恒 0: %+v", result.Results)
		}
	}
	if result.Tokens != 0 {
		t.Fatalf("PassThrough 用量应为 0: %d", result.Tokens)
	}
}
