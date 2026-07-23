package errors

import (
	stderrors "errors"
	"strings"
	"testing"
)

// TestNewCapturesStack 验证 New 捕获调用栈且包含本测试文件帧
func TestNewCapturesStack(t *testing.T) {
	err := New("stack anchor")
	stack := Stack(err)
	if !strings.Contains(stack, "errors_test.go") {
		t.Fatalf("expected stack to contain errors_test.go, got: %s", stack)
	}
}

// TestNewfCapturesStack 验证 Newf 格式化消息并捕获调用栈
func TestNewfCapturesStack(t *testing.T) {
	err := Newf("invalid status: %s", "UNKNOWN")
	if err.Error() != "invalid status: UNKNOWN" {
		t.Fatalf("unexpected message: %s", err.Error())
	}
	stack := Stack(err)
	if !strings.Contains(stack, "errors_test.go") {
		t.Fatalf("expected stack frame from Newf, got: %s", stack)
	}
}

// TestWrapCapturesStack 验证 Wrap 捕获调用栈且 Is 仍匹配根错误
func TestWrapCapturesStack(t *testing.T) {
	root := stderrors.New("gorm error")
	wrapped := Wrap(root, "查询会员失败")
	stack := Stack(wrapped)
	if !strings.Contains(stack, "errors_test.go") {
		t.Fatalf("expected stack to contain errors_test.go, got: %s", stack)
	}
	if !Is(wrapped, root) {
		t.Fatal("expected wrapped error to match root via Is")
	}
}

// TestWrapNil 验证 Wrap(nil, ...) 返回 nil
func TestWrapNil(t *testing.T) {
	if Wrap(nil, "ignored") != nil {
		t.Fatal("expected nil from Wrap(nil, ...)")
	}
}

// TestWrapFormattedMessage 验证 Wrap 支持格式化消息
func TestWrapFormattedMessage(t *testing.T) {
	root := stderrors.New("redis error")
	wrapped := Wrap(root, "连接 broker %s 失败", "localhost:9092")
	if !strings.Contains(wrapped.Error(), "连接 broker localhost:9092 失败") {
		t.Fatalf("unexpected message: %s", wrapped.Error())
	}
}

// TestStackNil 验证 Stack(nil) 返回空字符串
func TestStackNil(t *testing.T) {
	if Stack(nil) != "" {
		t.Fatal("expected empty stack for nil error")
	}
}

// TestNewNoStackHasNoUsefulStack 验证 NewNoStack 不捕获调用栈
func TestNewNoStackHasNoUsefulStack(t *testing.T) {
	err := NewNoStack("PLAIN_CODE")
	if err.Error() != "PLAIN_CODE" {
		t.Fatalf("unexpected message: %s", err.Error())
	}
	stack := Stack(err)
	if strings.Contains(stack, "errors_test.go") {
		t.Fatalf("NewNoStack should not capture call-site stack, got: %s", stack)
	}
}

// TestSentinelCapturesStackAndPreservesIdentity 验证 Sentinel 保留哨兵身份并捕获调用栈
func TestSentinelCapturesStackAndPreservesIdentity(t *testing.T) {
	base := NewNoStack("TOKEN_REQUIRED")
	err := Sentinel(base)
	if err.Error() != "TOKEN_REQUIRED" {
		t.Fatalf("Sentinel must preserve Error(): %s", err.Error())
	}
	if !Is(err, base) {
		t.Fatal("expected Is(Sentinel(base), base)")
	}
	stack := Stack(err)
	if !strings.Contains(stack, "errors_test.go") {
		t.Fatalf("expected call-site stack from Sentinel, got: %s", stack)
	}
	if strings.Contains(stack, "errors.go") && strings.Count(stack, "errors_test.go") == 0 {
		t.Fatalf("expected clean call-site stack, got: %s", stack)
	}
}

// TestSentinelNil 验证 Sentinel(nil) 返回 nil
func TestSentinelNil(t *testing.T) {
	if Sentinel(nil) != nil {
		t.Fatal("expected nil from Sentinel(nil)")
	}
}
