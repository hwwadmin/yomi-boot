package random

import "testing"

// TestBytes 验证随机字节生成、非全零及非法长度参数
func TestBytes(t *testing.T) {
	buf, err := Bytes(32)
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	if len(buf) != 32 {
		t.Fatalf("len=%d", len(buf))
	}
	allZero := true
	for _, b := range buf {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Fatal("随机字节不应全为 0")
	}
	if _, err := Bytes(0); err == nil {
		t.Fatal("n<=0 应失败")
	}
}

// TestHexAndURLSafe 验证 Hex 与 URLSafe 随机字符串生成
func TestHexAndURLSafe(t *testing.T) {
	h, err := Hex(8)
	if err != nil || len(h) != 16 {
		t.Fatalf("Hex: %q err=%v", h, err)
	}
	u, err := URLSafe(16)
	if err != nil || u == "" {
		t.Fatalf("URLSafe: %q err=%v", u, err)
	}
}
