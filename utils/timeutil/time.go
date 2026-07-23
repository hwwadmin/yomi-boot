// Package timeutil 时间与 Unix 毫秒时间戳互转的工具函数
//
// @author zwei
package timeutil

import "time"

// ToUnix 将 time.Time 转为 Unix 时间戳（毫秒）
func ToUnix(t time.Time) int64 {
	return t.UnixMilli()
}

// ToUnixPtr 将 *time.Time 转为 *int64 Unix 时间戳（毫秒），nil 入参返回 nil
func ToUnixPtr(t *time.Time) *int64 {
	if t == nil {
		return nil
	}
	v := t.UnixMilli()
	return &v
}
