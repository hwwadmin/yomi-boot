package md5

import "testing"

// TestSumHex 验证 MD5 十六进制摘要及原始长度
func TestSumHex(t *testing.T) {
	// echo -n "hello" | md5sum
	got := SumHex([]byte("hello"))
	want := "5d41402abc4b2a76b9719d911017c592"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	if len(Sum([]byte("hello"))) != 16 {
		t.Fatal("Sum 长度应为 16")
	}
}
