package passwd

import (
	"testing"
	"yt-boot/errors"
)

// TestHashAndVerify 验证密码哈希与校验及错误密码返回 ErrMismatch
func TestHashAndVerify(t *testing.T) {
	hash, err := Hash("secret-pass")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if err := Verify(hash, "secret-pass"); err != nil {
		t.Fatalf("Verify 应成功: %v", err)
	}
	if err := Verify(hash, "wrong"); !errors.Is(err, ErrMismatch) {
		t.Fatalf("期望 ErrMismatch，实际 %v", err)
	}
}

// TestHashWithCost 验证指定 cost 的密码哈希与校验
func TestHashWithCost(t *testing.T) {
	hash, err := HashWithCost("abc", 4) // MinCost 便于单测加速
	if err != nil {
		t.Fatalf("HashWithCost: %v", err)
	}
	if err := Verify(hash, "abc"); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}
