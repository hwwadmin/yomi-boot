package hmacsha

import "testing"

func TestHMACHexRejectsEmptyKey(t *testing.T) {
	if _, err := SHA1Hex(nil, []byte("a")); err == nil {
		t.Fatal("空密钥应拒绝")
	}
	if _, err := SHA256Hex(nil, nil); err == nil {
		t.Fatal("空密钥的 SHA256 hex 应失败")
	}
	if _, err := SHA512Hex(nil, []byte("a")); err == nil {
		t.Fatal("空密钥的 SHA512 hex 应失败")
	}
}
