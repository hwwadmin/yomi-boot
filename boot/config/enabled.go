// Package config 提供配置开关默认值能力
//
// @author zwei
package config

// BoolOrDefault 返回布尔配置值，未配置（nil）时默认 false
func BoolOrDefault(v *bool) bool {
	if v == nil {
		return false
	}
	return *v
}
