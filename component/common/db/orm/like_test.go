// LIKE 模式转义辅助的单元测试
//
// @author zwei

package orm

import "testing"

// TestLikeEscape 验证 LIKE 通配符转义
func TestLikeEscape(t *testing.T) {
	cases := []struct {
		value string
		want  string
	}{
		{value: "", want: ""},
		{value: "plain", want: "plain"},
		{value: `a\b`, want: `a\\b`},
		{value: "a%b", want: `a\%b`},
		{value: "a_b", want: `a\_b`},
		{value: `1%_2\3`, want: `1\%\_2\\3`},
		{value: "中文", want: "中文"},
	}
	for _, tc := range cases {
		if got := likeEscape(tc.value); got != tc.want {
			t.Fatalf("likeEscape(%q) 期望 %q，实际 %q", tc.value, tc.want, got)
		}
	}
}

// TestLikePatterns 验证左/右/全模糊模式的拼接与转义
func TestLikePatterns(t *testing.T) {
	if got := LikeLeft("a%"); got != `%a\%` {
		t.Fatalf("LikeLeft 期望 %q，实际 %q", `%a\%`, got)
	}
	if got := LikeRight("a_"); got != `a\_%` {
		t.Fatalf("LikeRight 期望 %q，实际 %q", `a\_%`, got)
	}
	if got := LikeFull(`a\b`); got != `%a\\b%` {
		t.Fatalf("LikeFull 期望 %q，实际 %q", `%a\\b%`, got)
	}
	if got := LikeFull(""); got != "%%" {
		t.Fatalf("LikeFull 空值期望 %q，实际 %q", "%%", got)
	}
}
