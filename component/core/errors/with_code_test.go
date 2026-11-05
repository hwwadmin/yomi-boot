package errors

import (
	stderrors "errors"
	"strings"
	"testing"
)

// TestWithCodeKeepsSentinelTextAndCause 验证对外文案是哨兵，原因链仍能匹配原始错误
func TestWithCodeKeepsSentinelTextAndCause(t *testing.T) {
	sentinel := NewNoStack("REQUEST_TIMEOUT")
	cause := stderrors.New("查询订单失败")
	err := WithCode(sentinel, cause)
	if err.Error() != "REQUEST_TIMEOUT" {
		t.Fatalf("对外文案应为哨兵，实际 %s", err.Error())
	}
	if !Is(err, sentinel) {
		t.Fatal("应匹配哨兵")
	}
	if !Is(err, cause) {
		t.Fatal("应匹配原始原因")
	}
	if !strings.Contains(Stack(err), "查询订单失败") {
		t.Fatalf("原因链应保留原始错误: %s", Stack(err))
	}
	gotSentinel, gotCause, ok := PublicCode(err)
	if !ok || gotCause != cause || gotSentinel.Error() != "REQUEST_TIMEOUT" {
		t.Fatalf("PublicCode 应返回哨兵与原因: ok=%v sentinel=%v cause=%v", ok, gotSentinel, gotCause)
	}
}

// TestWithCodeAlreadySentinel 验证 cause 已经是该哨兵时不再包装
func TestWithCodeAlreadySentinel(t *testing.T) {
	sentinel := NewNoStack("INVALID_BODY")
	once := WithCode(sentinel, stderrors.New("Key: 'Order.OrderID'"))
	twice := WithCode(sentinel, once)
	if twice != once {
		t.Fatal("重复收口应原样返回，避免丢掉原因")
	}
	plain := Sentinel(sentinel)
	if WithCode(sentinel, plain) != plain {
		t.Fatal("cause 本身已是该哨兵时应原样返回")
	}
}

// TestWithCodeNilArgs 验证空哨兵与空原因
func TestWithCodeNilArgs(t *testing.T) {
	cause := stderrors.New("boom")
	if WithCode(nil, cause) != cause {
		t.Fatal("哨兵为空时应返回 cause")
	}
	sentinel := NewNoStack("INTERNAL_ERROR")
	err := WithCode(sentinel, nil)
	if err.Error() != "INTERNAL_ERROR" || !Is(err, sentinel) {
		t.Fatalf("cause 为空时应等价于 Sentinel，实际 %v", err)
	}
	if _, _, ok := PublicCode(err); ok {
		t.Fatal("纯哨兵不应被 PublicCode 当成收口结果")
	}
	if _, _, ok := PublicCode(cause); ok {
		t.Fatal("普通错误不应被 PublicCode 当成收口结果")
	}
}
