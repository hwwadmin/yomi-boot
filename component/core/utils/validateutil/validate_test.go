package validateutil

import "testing"

func TestEmailAndPhone(t *testing.T) {
	if !IsValidEmail("user.name+tag@example.com") {
		t.Fatal("合法邮箱应通过")
	}
	if IsValidEmail("not-an-email") || IsValidEmail("") {
		t.Fatal("非法邮箱应失败")
	}
	if !IsValidPhone("13800138000") {
		t.Fatal("合法手机号应通过")
	}
	if IsValidPhone("12800138000") || IsValidPhone("1380013800") {
		t.Fatal("非法手机号应失败")
	}
}
