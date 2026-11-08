package passwd

import "testing"

func TestVerifyRejectsMalformedHash(t *testing.T) {
	if err := Verify("not-a-bcrypt-hash", "secret"); err == nil {
		t.Fatal("非法哈希应失败")
	}
}
