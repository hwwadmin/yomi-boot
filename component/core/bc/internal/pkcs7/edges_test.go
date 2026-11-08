package pkcs7

import "testing"

func TestPKCS7RejectsBadPadding(t *testing.T) {
	if _, err := Pad(nil, 0); err == nil {
		t.Fatal("非法块大小应失败")
	}
	if _, err := Unpad(nil, 16); err == nil {
		t.Fatal("空数据应失败")
	}
	if _, err := Unpad([]byte{0, 0}, 2); err == nil {
		t.Fatal("零填充应失败")
	}
	if _, err := Unpad([]byte{1, 2}, 2); err == nil {
		t.Fatal("不一致的填充应失败")
	}
}
