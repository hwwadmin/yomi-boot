package masking

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync/atomic"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// compiledRule 规则的编译形态：keys 预先小写，策略已解析
type compiledRule struct {
	keys     []string
	strategy string // 小写策略名；redact / keep_edges 特判
	fn       Strategy
	keepHead int
	keepTail int
}

// Service 脱敏服务；构造后只读，可并发调用。nil 接收者等价于全透传。
type Service struct {
	placeholder string
	exactMatch  bool
	rules       []compiledRule
}

// noopService 全透传共享实例
var noopService = &Service{}

// Noop 返回全透传脱敏服务
func Noop() *Service {
	return noopService
}

// New 校验配置并编译规则；策略名未注册时返回 error
func New(cfg Config) (*Service, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	svc := &Service{
		placeholder: strings.TrimSpace(cfg.Placeholder),
		exactMatch:  strings.EqualFold(strings.TrimSpace(cfg.Match), MatchExact),
		rules:       make([]compiledRule, 0, len(cfg.Rules)),
	}
	if svc.placeholder == "" {
		svc.placeholder = DefaultPlaceholder
	}
	for _, rule := range cfg.Rules {
		compiled := compiledRule{
			keys:     cleanKeys(rule.Keys),
			strategy: strings.ToLower(strings.TrimSpace(rule.Strategy)),
			keepHead: rule.KeepHead,
			keepTail: rule.KeepTail,
		}
		switch compiled.strategy {
		case StrategyRedact, StrategyKeepEdges:
			// 引擎内建解析，无需注册表
		default:
			fn, ok := lookupStrategy(compiled.strategy)
			if !ok {
				return nil, errors.Newf("masking: 未注册的策略: %s（可用: %s）",
					compiled.strategy, strings.Join(availableStrategyNames(), ", "))
			}
			compiled.fn = fn
		}
		svc.rules = append(svc.rules, compiled)
	}
	return svc, nil
}

// Active 是否存在生效规则
func (s *Service) Active() bool {
	return s != nil && len(s.rules) > 0
}

// ShouldMask 字段名是否命中规则（大小写不敏感、忽略首尾空白）
func (s *Service) ShouldMask(fieldName string) bool {
	return s.matchRule(fieldName) != nil
}

// Mask 按字段名应用策略；未命中规则、服务未启用或值为空时原样返回
func (s *Service) Mask(fieldName, value string) string {
	if value == "" {
		return value
	}
	rule := s.matchRule(fieldName)
	if rule == nil {
		return value
	}
	return s.applyRule(rule, value)
}

// MaskMap 递归脱敏 map（返回新 map；未启用时原样返回入参引用）
func (s *Service) MaskMap(m map[string]any) map[string]any {
	if !s.Active() || len(m) == 0 {
		return m
	}
	return s.maskAny(m).(map[string]any)
}

// MaskJSON 对 JSON 字符串按字段名递归脱敏；非 JSON 或解析失败时原样返回。
// UseNumber 避免大整数（如雪花 ID）经 float64 往返时精度丢失。
func (s *Service) MaskJSON(raw string) string {
	if !s.Active() {
		return raw
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || !looksLikeJSON(trimmed) {
		return raw
	}
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	decoder.UseNumber()
	var payload any
	if err := decoder.Decode(&payload); err != nil {
		return raw
	}
	data, err := json.Marshal(s.maskAny(payload))
	if err != nil {
		return raw
	}
	return string(data)
}

// MaskQuery 对 query string 中的敏感参数按策略脱敏；解析失败时原样返回
func (s *Service) MaskQuery(raw string) string {
	if !s.Active() || raw == "" {
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
		for i, item := range items {
			masked[i] = s.Mask(key, item)
		}
		values[key] = masked
	}
	return values.Encode()
}

// MaskHeaderMap 对 header map 按键名脱敏（key 建议已小写）；返回新 map
func (s *Service) MaskHeaderMap(headers map[string]string) map[string]string {
	if !s.Active() || len(headers) == 0 {
		return headers
	}
	out := make(map[string]string, len(headers))
	for k, v := range headers {
		out[k] = s.Mask(k, v)
	}
	return out
}

// MaskKV 对成对 key-value 脱敏（zap 风格日志字段）；奇数个视为调用方笔误，
// 仍处理前面的完整对，仅末尾落单的 key 原样保留。
// 未命中键的值若为 header map / map / JSON 字符串会递归处理。
func (s *Service) MaskKV(keysAndValues ...any) []any {
	if !s.Active() || len(keysAndValues) == 0 {
		return keysAndValues
	}
	pairEnd := len(keysAndValues) - len(keysAndValues)%2
	out := make([]any, len(keysAndValues))
	copy(out, keysAndValues)
	for i := 0; i < pairEnd; i += 2 {
		key, ok := out[i].(string)
		if !ok {
			key = fmt.Sprint(out[i])
		}
		if s.ShouldMask(key) {
			if text, isText := out[i+1].(string); isText {
				out[i+1] = s.Mask(key, text)
			} else {
				out[i+1] = s.placeholder
			}
			continue
		}
		out[i+1] = s.applyValue(out[i+1])
	}
	return out
}

var defaultService atomic.Pointer[Service]

// SetDefault 设置进程级默认脱敏服务；nil 视为透传。由 masking 插件在 Init 阶段调用。
func SetDefault(s *Service) {
	if s == nil {
		s = noopService
	}
	defaultService.Store(s)
}

// Current 返回进程级默认脱敏服务；未设置时返回透传实例
func Current() *Service {
	if s := defaultService.Load(); s != nil {
		return s
	}
	return noopService
}

// matchRule 返回首条命中字段名的规则
func (s *Service) matchRule(fieldName string) *compiledRule {
	if !s.Active() {
		return nil
	}
	name := strings.ToLower(strings.TrimSpace(fieldName))
	if name == "" {
		return nil
	}
	for i := range s.rules {
		for _, key := range s.rules[i].keys {
			if s.exactMatch {
				if name == key {
					return &s.rules[i]
				}
				continue
			}
			if strings.Contains(name, key) {
				return &s.rules[i]
			}
		}
	}
	return nil
}

// applyRule 应用单条规则；策略形状不符或返回原值时降级 placeholder
func (s *Service) applyRule(rule *compiledRule, value string) string {
	switch rule.strategy {
	case StrategyRedact:
		return s.placeholder
	case StrategyKeepEdges:
		runes := []rune(value)
		if rule.keepHead+rule.keepTail >= len(runes) {
			return s.placeholder
		}
		var b strings.Builder
		b.WriteString(string(runes[:rule.keepHead]))
		b.WriteString(strings.Repeat("*", len(runes)-rule.keepHead-rule.keepTail))
		b.WriteString(string(runes[len(runes)-rule.keepTail:]))
		return b.String()
	default:
		masked, ok := rule.fn(value)
		if !ok || masked == value {
			return s.placeholder
		}
		return masked
	}
}

// maskAny 递归脱敏 JSON 解析后的值
func (s *Service) maskAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		masked := make(map[string]any, len(typed))
		for key, item := range typed {
			masked[key] = s.maskEntry(key, item)
		}
		return masked
	case []any:
		masked := make([]any, len(typed))
		for i, item := range typed {
			masked[i] = s.maskAny(item)
		}
		return masked
	default:
		return value
	}
}

// maskEntry 处理 map 单个键值：命中规则按策略脱敏，非字符串值替换为 placeholder
func (s *Service) maskEntry(key string, value any) any {
	rule := s.matchRule(key)
	if rule == nil {
		return s.maskAny(value)
	}
	if text, ok := value.(string); ok {
		return s.applyRule(rule, text)
	}
	return s.placeholder
}

// applyValue 递归处理未命中键的 KV 值（header map / map / JSON 字符串）
func (s *Service) applyValue(value any) any {
	switch typed := value.(type) {
	case map[string]string:
		return s.MaskHeaderMap(typed)
	case map[string]any:
		return s.MaskMap(typed)
	case string:
		return s.MaskJSON(typed)
	default:
		return value
	}
}

// looksLikeJSON 粗判字符串是否为 JSON 对象/数组
func looksLikeJSON(s string) bool {
	return strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[")
}
