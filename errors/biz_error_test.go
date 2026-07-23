package errors

import "testing"

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
