package passwd

import (
	"strings"
	"testing"

	"gitee.com/hq_project/yomi-boot/errors"
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

// TestHashWithCost 验证指定 cost 的密码哈希与校验及越界成本拒绝
func TestHashWithCost(t *testing.T) {
	hash, err := HashWithCost("abc", 4) // MinCost 便于单测加速
	if err != nil {
		t.Fatalf("HashWithCost: %v", err)
	}
	if err := Verify(hash, "abc"); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if _, err := HashWithCost("abc", 3); err == nil {
		t.Fatal("cost 低于 MinCost 应失败")
	}
	if _, err := HashWithCost("abc", 32); err == nil {
		t.Fatal("cost 高于 MaxCost 应失败")
	}
}

// TestPasswordLengthLimit 验证 72 字节边界与超长密码统一拒绝
func TestPasswordLengthLimit(t *testing.T) {
	max := strings.Repeat("a", MaxPasswordBytes)
	hash, err := HashWithCost(max, 4)
	if err != nil {
		t.Fatalf("72 字节密码应可哈希: %v", err)
	}
	if err := Verify(hash, max); err != nil {
		t.Fatalf("72 字节密码应可校验: %v", err)
	}

	for _, plain := range []string{max + "b", strings.Repeat("a", 1000)} {
		if _, err := Hash(plain); !errors.Is(err, ErrPasswordTooLong) {
			t.Fatalf("超长密码 Hash 期望 ErrPasswordTooLong，实际 %v", err)
		}
		if err := Verify(hash, plain); !errors.Is(err, ErrPasswordTooLong) {
			t.Fatalf("超长密码 Verify 期望 ErrPasswordTooLong，实际 %v", err)
		}
	}
}

// TestCostAndNeedsRehash 验证成本读取、重哈希判断与非法输入
func TestCostAndNeedsRehash(t *testing.T) {
	hash, err := HashWithCost("abc", 4)
	if err != nil {
		t.Fatal(err)
	}
	cost, err := Cost(hash)
	if err != nil || cost != 4 {
		t.Fatalf("Cost got %d err=%v", cost, err)
	}
	need, err := NeedsRehash(hash, 10)
	if err != nil || !need {
		t.Fatalf("低于目标成本应返回 true: need=%v err=%v", need, err)
	}
	need, err = NeedsRehash(hash, 4)
	if err != nil || need {
		t.Fatalf("等于目标成本应返回 false: need=%v err=%v", need, err)
	}
	if _, err := NeedsRehash(hash, 3); err == nil {
		t.Fatal("非法目标成本应失败")
	}
	if _, err := NeedsRehash("not a bcrypt hash", 10); err == nil {
		t.Fatal("非法哈希应失败")
	}
}
