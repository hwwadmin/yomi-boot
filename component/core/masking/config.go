package masking

import (
	"strings"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// 策略名常量；redact / keep_edges 由引擎内建解析，其余走注册表
const (
	StrategyRedact    = "redact"     // 整值替换为 placeholder
	StrategyKeepEdges = "keep_edges" // 保留头尾、中段替换为星号（参数走规则配置）
	StrategyPhone     = "phone"      // 大陆手机号：前 3 后 4，如 138****5678
	StrategyIDCard    = "idcard"     // 身份证号：前 6 后 4
	StrategyBankCard  = "bankcard"   // 银行卡号：仅保留后 4 位
	StrategyEmail     = "email"      // 邮箱：本地部分保留首字符，域名保留
	StrategyName      = "name"       // 姓名：保留首字符 + 定长 **（不泄露长度）
)

// 字段名匹配方式
const (
	MatchContains = "contains" // 字段名包含即命中（默认）
	MatchExact    = "exact"    // 字段名全等才命中
)

// DefaultPlaceholder redact 策略的默认占位符
const DefaultPlaceholder = "***"

// RuleConfig 单条脱敏规则：命中 Keys 的字段应用 Strategy
type RuleConfig struct {
	Keys     []string // Keys 是敏感字段名集合（大小写不敏感，按 Config.Match 匹配）
	Strategy string   // Strategy 是策略名：redact / keep_edges / 内置或已注册策略
	KeepHead int      // KeepHead 是 keep_edges 保留的前缀字符数（rune 计）；仅 keep_edges 可配
	KeepTail int      // KeepTail 是 keep_edges 保留的后缀字符数（rune 计）；仅 keep_edges 可配
}

// Config 脱敏服务配置
type Config struct {
	Placeholder string       // Placeholder 是 redact 占位符；空默认 ***
	Match       string       // Match 是字段名匹配方式：contains（默认）/ exact
	Rules       []RuleConfig // Rules 按序生效，首条命中即应用；空=不脱敏
}

// Validate 校验配置语义；策略名是否已注册在 New 中检查
func (c Config) Validate() error {
	match := strings.ToLower(strings.TrimSpace(c.Match))
	if match != "" && match != MatchContains && match != MatchExact {
		return errors.Newf("masking: match 仅支持 %s 或 %s", MatchContains, MatchExact)
	}
	for i, rule := range c.Rules {
		strategy := strings.ToLower(strings.TrimSpace(rule.Strategy))
		if strategy == "" {
			return errors.Newf("masking: rules[%d].strategy 不能为空", i)
		}
		if len(cleanKeys(rule.Keys)) == 0 {
			return errors.Newf("masking: rules[%d].keys 不能为空", i)
		}
		if rule.KeepHead < 0 || rule.KeepTail < 0 {
			return errors.Newf("masking: rules[%d].keep_head/keep_tail 不能为负", i)
		}
		if strategy != StrategyKeepEdges && (rule.KeepHead != 0 || rule.KeepTail != 0) {
			return errors.Newf("masking: rules[%d] 仅 keep_edges 可配置 keep_head/keep_tail", i)
		}
		if strategy == StrategyKeepEdges && rule.KeepHead+rule.KeepTail <= 0 {
			return errors.Newf("masking: rules[%d] keep_edges 的 keep_head+keep_tail 须 > 0", i)
		}
	}
	return nil
}

// cleanKeys 清洗字段名：去空白、转小写、丢弃空项
func cleanKeys(keys []string) []string {
	cleaned := make([]string, 0, len(keys))
	for _, k := range keys {
		k = strings.ToLower(strings.TrimSpace(k))
		if k == "" {
			continue
		}
		cleaned = append(cleaned, k)
	}
	return cleaned
}
