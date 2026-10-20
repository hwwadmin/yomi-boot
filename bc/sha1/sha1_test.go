package sha1

import "testing"

// TestSumHex 验证 SHA-1 标准测试向量及原始长度
func TestSumHex(t *testing.T) {
	// echo -n "abc" | sha1sum
	got := SumHex([]byte("abc"))
	want := "a9993e364706816aba3e25717850c26c9cd0d89d"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	if len(Sum([]byte("abc"))) != 20 {
		t.Fatal("SHA1 长度应为 20")
	}
}
