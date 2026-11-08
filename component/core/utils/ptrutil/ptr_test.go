package ptrutil

import "testing"

func TestPtrHelpers(t *testing.T) {
	if PtrFromString("  ") != nil {
		t.Fatal("空白字符串应返回 nil")
	}
	got := PtrFromString(" yomi ")
	if got == nil || *got != "yomi" {
		t.Fatalf("PtrFromString = %v", got)
	}
	if StringFromPtr(nil) != "" {
		t.Fatal("nil 指针应返回空字符串")
	}
	value := "ok"
	if StringFromPtr(&value) != "ok" {
		t.Fatal("应解引用字符串")
	}
	if PtrFromInt64(0) != nil || PtrFromInt64(-3) != nil {
		t.Fatal("非正数应返回 nil")
	}
	n := PtrFromInt64(8)
	if n == nil || *n != 8 {
		t.Fatalf("PtrFromInt64 = %v", n)
	}
	zero := Ptr(0)
	if zero == nil || *zero != 0 {
		t.Fatal("Ptr 应保留零值")
	}
}
