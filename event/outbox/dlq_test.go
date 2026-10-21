// Package outbox 的死信错误信息测试
//
// @author zwei
package outbox

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestCoreErrorMessage 验证死信错误信息按字符截断且不破坏 UTF-8
func TestCoreErrorMessage(t *testing.T) {
	if got := coreErrorMessage(nil); got != "" {
		t.Fatalf("nil error 期望空串，实际 %q", got)
	}

	short := errors.New("boom")
	if got := coreErrorMessage(short); got != "boom" {
		t.Fatalf("短错误期望原样返回，实际 %q", got)
	}

	exact := errors.New(strings.Repeat("错", dlqErrorMessageLimit))
	if got := coreErrorMessage(exact); got != exact.Error() {
		t.Fatalf("未超限错误不应截断，实际长度 %d", utf8.RuneCountInString(got))
	}

	long := errors.New(strings.Repeat("错", dlqErrorMessageLimit+100))
	got := coreErrorMessage(long)
	if !utf8.ValidString(got) {
		t.Fatal("截断后的错误信息不是合法 UTF-8")
	}
	if !strings.HasSuffix(got, dlqErrorMessageSuffix) {
		t.Fatalf("截断后的错误信息缺少截断标记: %q", got)
	}
	wantLen := dlqErrorMessageLimit + utf8.RuneCountInString(dlqErrorMessageSuffix)
	if utf8.RuneCountInString(got) != wantLen {
		t.Fatalf("截断长度期望 %d，实际 %d", wantLen, utf8.RuneCountInString(got))
	}
}
