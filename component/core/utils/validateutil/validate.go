// Package validateutil 常见格式（邮箱、手机号）校验工具函数
//
// @author zwei
package validateutil

import "regexp"

// emailRegex 是邮箱格式的正则表达式
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// phoneRegex 是中国大陆手机号格式的正则表达式（1 开头的 11 位数字）
var phoneRegex = regexp.MustCompile(`^1[3-9]\d{9}$`)

// IsValidEmail 校验邮箱格式是否合法
func IsValidEmail(email string) bool {
	return emailRegex.MatchString(email)
}

// IsValidPhone 校验中国大陆手机号格式是否合法
func IsValidPhone(phone string) bool {
	return phoneRegex.MatchString(phone)
}
