package errors

import (
	"testing"
)

// TestNewBiz 验证 NewBiz 构造业务错误
func TestNewBiz(t *testing.T) {
	err := NewBiz("INVALID_INPUT", "请求参数不合法")
	if err.Code != "INVALID_INPUT" || err.Message != "请求参数不合法" {
		t.Fatalf("unexpected biz error: %+v", err)
	}
}

// TestBizErrorIsMatchesByCode 验证 Is 按错误码匹配 BizError
func TestBizErrorIsMatchesByCode(t *testing.T) {
	left := NewBiz("TOKEN_INVALID", "令牌无效或已过期")
	right := NewBiz("TOKEN_INVALID", "令牌无效或已过期")
	if !Is(left, right) {
		t.Fatal("expected BizError Is to match by code")
	}
}

func TestBizErrorEdges(t *testing.T) {
	var nilErr *BizError
	if nilErr.Error() != "" || nilErr.Unwrap() != nil || nilErr.Is(NewBiz("X", "")) {
		t.Fatal("nil BizError 应为空错误")
	}
	if got := NewBiz("ONLY_CODE", "").Error(); got != "[ONLY_CODE]" {
		t.Fatalf("无消息 Error = %q", got)
	}
	cause := New("root")
	wrapped := WrapBiz(cause, "WRAPPED", "包装")
	if wrapped.Unwrap() != cause {
		t.Fatal("WrapBiz 应保留 cause")
	}
	if !Is(wrapped, NewBiz("WRAPPED", "其他文案")) {
		t.Fatal("Is 应按 code 匹配")
	}
	formatted := NewBizf("FORMATTED", "n=%d", 3)
	if formatted.Message != "n=3" || formatted.Error() != "[FORMATTED] n=3" {
		t.Fatalf("NewBizf = %+v", formatted)
	}
	got, ok := AsType[*BizError](error(formatted))
	if !ok || got.Code != "FORMATTED" {
		t.Fatalf("AsType = %+v ok=%v", got, ok)
	}
}

func TestCodeWithCauseNilEdges(t *testing.T) {
	var collapsed *codeWithCause
	if collapsed.Error() != "" {
		t.Fatal("nil codeWithCause.Error 应为空")
	}
	collapsed = &codeWithCause{sentinel: NewNoStack("CODE")}
	if collapsed.Error() != "CODE" {
		t.Fatalf("Error = %q", collapsed.Error())
	}
	if got := collapsed.Unwrap(); len(got) != 1 || got[0].Error() != "CODE" {
		t.Fatalf("无 cause 的 Unwrap = %v", got)
	}
}
