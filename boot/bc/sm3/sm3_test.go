package sm3

import "testing"

// TestSumHex 验证 SM3 标准测试向量及原始长度
func TestSumHex(t *testing.T) {
	// GB/T 32905-2016 示例 1：SM3("abc")
	got := SumHex([]byte("abc"))
	want := "66c7f0f462eeedd9d1f2d46bdc10e4e24167c4875cf2f7a2297da02b8f4ba8e0"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	if len(Sum([]byte("abc"))) != Size {
		t.Fatalf("SM3 长度应为 %d", Size)
	}
}
