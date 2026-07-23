package hmac

import (
	"encoding/hex"
	"testing"
)

// TestSHA256 验证 HMAC-SHA256 摘要、Hex 输出及空密钥校验
func TestSHA256(t *testing.T) {
	// RFC 4231 case 1 truncated conceptually — verify round consistency
	sum, err := SHA256([]byte("key"), []byte("data"))
	if err != nil || len(sum) != 32 {
		t.Fatalf("SHA256: len=%d err=%v", len(sum), err)
	}
	hexSum, err := SHA256Hex([]byte("key"), []byte("data"))
	if err != nil || hexSum != hex.EncodeToString(sum) {
		t.Fatalf("SHA256Hex mismatch: %s err=%v", hexSum, err)
	}
	if _, err := SHA256(nil, []byte("x")); err == nil {
		t.Fatal("空 key 应失败")
	}
}

// TestEqual 验证常量时间比较行为
func TestEqual(t *testing.T) {
	a := []byte{1, 2, 3}
	b := []byte{1, 2, 3}
	c := []byte{1, 2, 4}
	if !Equal(a, b) || Equal(a, c) {
		t.Fatal("Equal 行为异常")
	}
}
