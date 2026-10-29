// 公共入参校验单测。
//
// @author zwei

package vectordb

import (
	"context"
	"strings"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// TestNormalizeCollectionName 覆盖修剪、合法字符与长度边界
func TestNormalizeCollectionName(t *testing.T) {
	t.Parallel()
	valid255 := "a" + strings.Repeat("b", maxNameLen-1)
	cases := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{"trim", "  coll_1 ", "coll_1", true},
		{"single letter", "a", "a", true},
		{"underscore suffix", "a_1", "a_1", true},
		{"max length", valid255, valid255, true},
		{"empty", "", "", false},
		{"blank", "   ", "", false},
		{"digit prefix", "1abc", "", false},
		{"dash", "a-b", "", false},
		{"too long", valid255 + "b", "", false},
		{"cjk", "集合", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeCollectionName(tc.in)
			if tc.ok {
				if err != nil || got != tc.want {
					t.Fatalf("got=%q err=%v", got, err)
				}
				return
			}
			if !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

// TestValidateDimensionRange 覆盖维度上下界
func TestValidateDimensionRange(t *testing.T) {
	t.Parallel()
	bad := []int{0, -1, maxDimension + 1}
	for _, d := range bad {
		if err := validateDimension(d); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("dim %d err=%v", d, err)
		}
	}
	for _, d := range []int{1, maxDimension} {
		if err := validateDimension(d); err != nil {
			t.Fatalf("dim %d err=%v", d, err)
		}
	}
}

// TestValidateRecordsInput 覆盖记录、向量、文本与元数据校验
func TestValidateRecordsInput(t *testing.T) {
	t.Parallel()
	longID := strings.Repeat("a", maxIDLen+1)
	longText := strings.Repeat("a", maxTextLen+1)
	cases := []struct {
		name    string
		records []Record
		dim     int
		ok      bool
	}{
		{"empty", nil, 2, false},
		{"valid", []Record{{ID: "a", Vector: []float32{1, 0}}}, 2, true},
		{"trim id", []Record{{ID: " a ", Vector: []float32{1, 0}}}, 2, true},
		{"blank id", []Record{{ID: " ", Vector: []float32{1, 0}}}, 2, false},
		{"long id", []Record{{ID: longID, Vector: []float32{1, 0}}}, 2, false},
		{"duplicate id", []Record{{ID: "a", Vector: []float32{1, 0}}, {ID: " a ", Vector: []float32{0, 1}}}, 2, false},
		{"wrong dim", []Record{{ID: "a", Vector: []float32{1}}}, 2, false},
		{"long text", []Record{{ID: "a", Vector: []float32{1, 0}, Text: longText}}, 2, false},
		{"blank metadata key", []Record{{ID: "a", Vector: []float32{1, 0}, Metadata: map[string]string{" ": "v"}}}, 2, false},
		{"quoted metadata key", []Record{{ID: "a", Vector: []float32{1, 0}, Metadata: map[string]string{`a"b`: "v"}}}, 2, false},
		{"backslash metadata key", []Record{{ID: "a", Vector: []float32{1, 0}, Metadata: map[string]string{`a\b`: "v"}}}, 2, false},
		{"duplicate trimmed metadata key", []Record{{ID: "a", Vector: []float32{1, 0}, Metadata: map[string]string{"k": "1", " k ": "2"}}}, 2, false},
		{"nil metadata", []Record{{ID: "a", Vector: []float32{1, 0}, Metadata: nil}}, 2, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRecords(tc.records, tc.dim)
			if tc.ok && err != nil {
				t.Fatalf("err=%v", err)
			}
			if !tc.ok && !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

// TestValidateIDsInput 覆盖主键列表校验
func TestValidateIDsInput(t *testing.T) {
	t.Parallel()
	if err := validateIDs(nil); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("nil err=%v", err)
	}
	if err := validateIDs([]string{" "}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("blank err=%v", err)
	}
	if err := validateIDs([]string{strings.Repeat("a", maxIDLen+1)}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("long err=%v", err)
	}
	if err := validateIDs([]string{" a ", "b"}); err != nil {
		t.Fatalf("valid err=%v", err)
	}
}

// TestValidateFilterInput 覆盖过滤条件键校验
func TestValidateFilterInput(t *testing.T) {
	t.Parallel()
	if err := validateFilter(nil, true); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("required empty err=%v", err)
	}
	if err := validateFilter(nil, false); err != nil {
		t.Fatalf("optional empty err=%v", err)
	}
	for _, filter := range []map[string]string{
		{" ": "v"},
		{`a"b`: "v"},
		{`a\b`: "v"},
	} {
		if err := validateFilter(filter, true); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("filter=%v err=%v", filter, err)
		}
	}
	if err := validateFilter(map[string]string{" k ": "v"}, true); err != nil {
		t.Fatalf("valid err=%v", err)
	}
}

// TestValidateSearchQueryInput 覆盖向量、文本、TopK 与过滤校验
func TestValidateSearchQueryInput(t *testing.T) {
	t.Parallel()
	longText := strings.Repeat("a", maxTextLen+1)
	cases := []struct {
		name string
		q    SearchQuery
		ok   bool
	}{
		{"valid", SearchQuery{Vector: []float32{1, 0}, TopK: 1}, true},
		{"wrong dim", SearchQuery{Vector: []float32{1}, TopK: 1}, false},
		{"long text", SearchQuery{Vector: []float32{1, 0}, TopK: 1, Text: longText}, false},
		{"zero topk", SearchQuery{Vector: []float32{1, 0}, TopK: 0}, false},
		{"negative topk", SearchQuery{Vector: []float32{1, 0}, TopK: -1}, false},
		{"too large topk", SearchQuery{Vector: []float32{1, 0}, TopK: MaxTopK + 1}, false},
		{"max topk", SearchQuery{Vector: []float32{1, 0}, TopK: MaxTopK}, true},
		{"bad filter", SearchQuery{Vector: []float32{1, 0}, TopK: 1, Filter: map[string]string{`a"b`: "v"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSearchQuery(tc.q, 2)
			if tc.ok && err != nil {
				t.Fatalf("err=%v", err)
			}
			if !tc.ok && !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

// nilContext 返回 nil 值的 context.Context，用于覆盖库对空上下文的兜底契约
func nilContext() context.Context { return nil }

// TestWithTimeoutNilContext 验证空上下文回退 Background 并套默认超时
func TestWithTimeoutNilContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := withTimeout(nilContext(), 0)
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("missing deadline")
	}
	if remaining := time.Until(deadline); remaining <= 0 || remaining > defaultTimeout+time.Second {
		t.Fatalf("remaining=%v", remaining)
	}
	ctx2, cancel2 := withTimeout(context.Background(), time.Minute)
	defer cancel2()
	deadline2, ok := ctx2.Deadline()
	if !ok || time.Until(deadline2) <= 30*time.Second {
		t.Fatalf("custom timeout not applied: %v", deadline2)
	}
}

// TestCopyAndMatchHelpers 覆盖元数据/向量拷贝与过滤匹配
func TestCopyAndMatchHelpers(t *testing.T) {
	t.Parallel()
	if copyMetadata(nil) != nil || copyMetadata(map[string]string{}) != nil {
		t.Fatal("empty metadata should return nil")
	}
	src := map[string]string{" k ": "v"}
	dst := copyMetadata(src)
	if dst["k"] != "v" || len(dst) != 1 {
		t.Fatalf("copy=%v", dst)
	}
	dst["k"] = "changed"
	if src[" k "] != "v" {
		t.Fatal("copy shares source map")
	}
	if copyVector(nil) != nil {
		t.Fatal("nil vector should stay nil")
	}
	vec := []float32{1, 2}
	copied := copyVector(vec)
	vec[0] = 9
	if copied[0] != 1 {
		t.Fatal("copy shares source slice")
	}
	if !matchFilter(nil, nil) || !matchFilter(map[string]string{"k": "v"}, nil) {
		t.Fatal("empty filter should match")
	}
	if !matchFilter(map[string]string{"k": "v"}, map[string]string{" k ": "v"}) {
		t.Fatal("trimmed key should match")
	}
	if matchFilter(map[string]string{"k": "v"}, map[string]string{"k": "x"}) {
		t.Fatal("value mismatch should not match")
	}
}
