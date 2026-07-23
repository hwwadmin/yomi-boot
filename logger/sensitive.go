// Package logger 文件日志统一 sensitive 语义
//
// 仅由 log.sensitive_keys / log.sensitive_match 驱动；keys 为空则不脱敏。
//
// @author zwei
package logger

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"yomi-boot/errors"
)

const (
	// SensitiveMatchContains 字段名包含即命中（默认）
	SensitiveMatchContains = "contains"
	// SensitiveMatchExact 字段名全等才命中
	SensitiveMatchExact = "exact"

	sensitivePlaceholder = "[REDACTED]"
)

// Sensitive 文件日志敏感字段处理器（access / info / error / debug 共用）
type Sensitive struct {
	match string
	keys  []string
}

// NewSensitive 创建 sensitive 处理器；match 空则 contains；非法 match 返回 error
func NewSensitive(match string, keys []string) (*Sensitive, error) {
	match = strings.ToLower(strings.TrimSpace(match))
	if match == "" {
		match = SensitiveMatchContains
	}
	if match != SensitiveMatchContains && match != SensitiveMatchExact {
		return nil, errors.Newf("log.sensitive_match 仅支持 %s 或 %s", SensitiveMatchContains, SensitiveMatchExact)
	}
	cleaned := make([]string, 0, len(keys))
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		cleaned = append(cleaned, k)
	}
	return &Sensitive{match: match, keys: cleaned}, nil
}

// emptySensitive 无 keys 的透传实例
func emptySensitive() *Sensitive {
	return &Sensitive{match: SensitiveMatchContains, keys: nil}
}

// active 判断是否启用脱敏（非 nil 且 keys 非空）
func (s *Sensitive) active() bool {
	return s != nil && len(s.keys) > 0
}

// ShouldMask 判断字段名是否命中 sensitive_keys（大小写不敏感）
func (s *Sensitive) ShouldMask(fieldName string) bool {
	if !s.active() {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(fieldName))
	if name == "" {
		return false
	}
	for _, sk := range s.keys {
		sk = strings.ToLower(strings.TrimSpace(sk))
		if sk == "" {
			continue
		}
		if s.match == SensitiveMatchExact {
			if name == sk {
				return true
			}
			continue
		}
		if strings.Contains(name, sk) {
			return true
		}
	}
	return false
}

// ApplyKV 对成对 key-value 按字段名脱敏；命中则 value 为 [REDACTED]，map/JSON 递归
func (s *Sensitive) ApplyKV(keysAndValues ...any) []any {
	if !s.active() || len(keysAndValues) == 0 {
		return keysAndValues
	}
	out := make([]any, len(keysAndValues))
	copy(out, keysAndValues)
	if len(out)%2 != 0 {
		return out
	}
	for i := 0; i < len(out); i += 2 {
		key, ok := out[i].(string)
		if !ok {
			key = fmt.Sprint(out[i])
		}
		if s.ShouldMask(key) {
			out[i+1] = sensitivePlaceholder
			continue
		}
		out[i+1] = s.applyValue(out[i+1])
	}
	return out
}

// ApplyJSON 对 JSON 字符串按 key 递归脱敏；非 JSON 或解析失败返回原文
func (s *Sensitive) ApplyJSON(raw string) string {
	if !s.active() {
		return raw
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || !looksLikeJSON(trimmed) {
		return raw
	}
	var payload any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return raw
	}
	masked := s.maskJSONValue(payload)
	data, err := json.Marshal(masked)
	if err != nil {
		return raw
	}
	return string(data)
}

// ApplyQuery 对 query string 中的敏感参数脱敏
func (s *Sensitive) ApplyQuery(raw string) string {
	if raw == "" || !s.active() {
		return raw
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return raw
	}
	for key, items := range values {
		if !s.ShouldMask(key) {
			continue
		}
		masked := make([]string, len(items))
		for i := range items {
			masked[i] = sensitivePlaceholder
		}
		values[key] = masked
	}
	return values.Encode()
}

// ApplyHeaderMap 对已扁平化的 header map 脱敏（key 建议已小写）
func (s *Sensitive) ApplyHeaderMap(headers map[string]string) map[string]string {
	if len(headers) == 0 || !s.active() {
		return headers
	}
	out := make(map[string]string, len(headers))
	for k, v := range headers {
		if s.ShouldMask(k) {
			out[k] = sensitivePlaceholder
			continue
		}
		out[k] = v
	}
	return out
}

// applyValue 递归处理 KV 值（map/JSON 字符串）
func (s *Sensitive) applyValue(value any) any {
	switch typed := value.(type) {
	case map[string]string:
		return s.ApplyHeaderMap(typed)
	case map[string]any:
		return s.maskJSONValue(typed)
	case string:
		return s.ApplyJSON(typed)
	default:
		return value
	}
}

// maskJSONValue 递归脱敏 JSON 解析后的值
func (s *Sensitive) maskJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		masked := make(map[string]any, len(typed))
		for key, item := range typed {
			if s.ShouldMask(key) {
				masked[key] = sensitivePlaceholder
				continue
			}
			masked[key] = s.maskJSONValue(item)
		}
		return masked
	case []any:
		masked := make([]any, len(typed))
		for i, item := range typed {
			masked[i] = s.maskJSONValue(item)
		}
		return masked
	default:
		return value
	}
}
