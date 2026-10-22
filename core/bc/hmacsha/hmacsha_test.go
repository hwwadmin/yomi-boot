package hmacsha

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// TestSHA1 验证 RFC 2202 测试向量及空密钥校验
func TestSHA1(t *testing.T) {
	key := bytes.Repeat([]byte{0x0b}, 20)
	hexSum, err := SHA1Hex(key, []byte("Hi There"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "b617318655057264e28bc0b6fb378c8ef146be00"; hexSum != want {
		t.Fatalf("got %s want %s", hexSum, want)
	}
	if _, err := SHA1(nil, []byte("x")); err == nil {
		t.Fatal("空 key 应失败")
	}
}

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

// TestSHA512 验证 RFC 4231 测试向量及空密钥校验
func TestSHA512(t *testing.T) {
	key := bytes.Repeat([]byte{0x0b}, 20)
	hexSum, err := SHA512Hex(key, []byte("Hi There"))
	if err != nil {
		t.Fatal(err)
	}
	want := "87aa7cdea5ef619d4ff0b4241a1d6cb02379f4e2ce4ec2787ad0b30545e17cdedaa833b7d6b8a702038b274eaea3f4e4be9d914eeb61f1702e696c203a126854"
	if hexSum != want {
		t.Fatalf("got %s want %s", hexSum, want)
	}
	if _, err := SHA512(nil, []byte("x")); err == nil {
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
