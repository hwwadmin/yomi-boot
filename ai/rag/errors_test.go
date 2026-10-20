// 哨兵包装与错误链单测
//
// @author zwei

package rag

import (
	"strings"
	"testing"

	"gitee.com/hq_project/yomi-boot/errors"
)

// TestWrapSentinelKeepsCleanCode 验证包装后 Error() 仍是干净哨兵码，原因保留在错误链
func TestWrapSentinelKeepsCleanCode(t *testing.T) {
	t.Parallel()
	err := wrapSentinel(ErrBadResponse, errors.New("detail"))
	if err.Error() != "AI_RAG_BAD_RESPONSE" {
		t.Fatalf("Error()=%q", err.Error())
	}
	if !errors.Is(err, ErrBadResponse) {
		t.Fatal("errors.Is 未匹配 ErrBadResponse")
	}
	if stack := errors.Stack(err); !strings.Contains(stack, "detail") {
		t.Fatalf("Stack 丢失原因: %q", stack)
	}
}
