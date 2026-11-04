// 哨兵包装与错误链单测
//
// @author zwei

package rag

import (
	"strings"
	"testing"

	"github.com/hwwadmin/yomi-boot/component/core/errors"
)

// TestWrapSentinel 验证哨兵与原因同时保留；错误链风格与同族四包一致
func TestWrapSentinel(t *testing.T) {
	t.Parallel()
	only := wrapSentinel(ErrBadResponse, nil)
	if !errors.Is(only, ErrBadResponse) {
		t.Fatalf("err=%v", only)
	}
	cause := errors.New("detail")
	err := wrapSentinel(ErrBadResponse, cause)
	if !errors.Is(err, ErrBadResponse) || !errors.Is(err, cause) {
		t.Fatalf("err=%v", err)
	}
	if stack := errors.Stack(err); !strings.Contains(stack, "detail") {
		t.Fatalf("Stack 丢失原因: %q", stack)
	}
	if !strings.Contains(err.Error(), "AI_RAG_BAD_RESPONSE") || !strings.Contains(err.Error(), "detail") {
		t.Fatalf("Error() 应与同族四包一致携带原因文本: %q", err.Error())
	}
}
