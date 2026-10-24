// LIKE 模式转义辅助
//
// @author zwei

package orm

import "strings"

// likeEscape 对 LIKE 模式中的特殊字符进行转义
// 数据库 LIKE 默认使用 \ 作为转义字符，需转义 \、%、_ 三个字符
func likeEscape(value string) string {
	if value == "" {
		return ""
	}

	var builder strings.Builder
	builder.Grow(len(value))

	for _, r := range value {
		switch r {
		case '\\':
			builder.WriteString("\\\\")
		case '%':
			builder.WriteString("\\%")
		case '_':
			builder.WriteString("\\_")
		default:
			builder.WriteRune(r)
		}
	}

	return builder.String()
}

// LikeLeft 拼接左模糊 LIKE 模式：%value
// 返回的字符串已对 LIKE 通配符做转义，可直接作为参数化查询的值
func LikeLeft(value string) string {
	return "%" + likeEscape(value)
}

// LikeRight 拼接右模糊 LIKE 模式：value%
// 返回的字符串已对 LIKE 通配符做转义，可直接作为参数化查询的值
func LikeRight(value string) string {
	return likeEscape(value) + "%"
}

// LikeFull 拼接全模糊 LIKE 模式：%value%
// 返回的字符串已对 LIKE 通配符做转义，可直接作为参数化查询的值
func LikeFull(value string) string {
	return "%" + likeEscape(value) + "%"
}
