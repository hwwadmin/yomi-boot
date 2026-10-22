// Package ptrutil 指针与基础类型互转的工具函数
//
// @author zwei
package ptrutil

import "strings"

// PtrFromString 将字符串转为指针，空字符串返回 nil
func PtrFromString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

// StringFromPtr 将字符串指针安全解引用，nil 返回空字符串
func StringFromPtr(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

// PtrFromInt64 将 int64 转为指针，非正数返回 nil
func PtrFromInt64(value int64) *int64 {
	if value <= 0 {
		return nil
	}

	return &value
}

// Ptr 返回指向 v 的指针；零值仍返回非空指针，与 PtrFromInt64 的非正数回 nil 不同
func Ptr[T any](v T) *T {
	return &v
}
