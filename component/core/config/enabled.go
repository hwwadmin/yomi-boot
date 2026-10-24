package config

// BoolOr 返回布尔配置值，未配置（nil）时返回 def
func BoolOr(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

// BoolOrDefault 返回布尔配置值，未配置（nil）时默认 false
func BoolOrDefault(v *bool) bool {
	return BoolOr(v, false)
}
