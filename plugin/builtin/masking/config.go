// Package maskingplugin 统一脱敏服务插件
//
// @author zwei
package maskingplugin

import (
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/masking"
)

// SectionKey YAML 根 key
const SectionKey = "masking"

// RuleBootConfig 单条脱敏规则的 YAML 绑定
type RuleBootConfig struct {
	Keys     []string `mapstructure:"keys"`      // 敏感字段名集合（大小写不敏感，按 match 匹配）
	Strategy string   `mapstructure:"strategy"`  // 策略名：redact / keep_edges / 内置或已注册策略
	KeepHead int      `mapstructure:"keep_head"` // 仅 keep_edges：保留前缀字符数（rune 计）
	KeepTail int      `mapstructure:"keep_tail"` // 仅 keep_edges：保留后缀字符数（rune 计）
}

// BootConfig masking 配置段的 YAML 绑定
type BootConfig struct {
	Placeholder string           `mapstructure:"placeholder"` // redact 占位符，默认 ***
	Match       string           `mapstructure:"match"`       // contains（默认）/ exact
	Rules       []RuleBootConfig `mapstructure:"rules"`       // 按序首条命中生效；空=不脱敏（引擎零内置规则）
}

// ToConfig 转换为运行时配置
func (c BootConfig) ToConfig() masking.Config {
	rules := make([]masking.RuleConfig, 0, len(c.Rules))
	for _, rule := range c.Rules {
		rules = append(rules, masking.RuleConfig{
			Keys:     rule.Keys,
			Strategy: rule.Strategy,
			KeepHead: rule.KeepHead,
			KeepTail: rule.KeepTail,
		})
	}
	return masking.Config{
		Placeholder: c.Placeholder,
		Match:       c.Match,
		Rules:       rules,
	}
}

// Validate 校验 masking 配置段语义（含策略名是否已注册）
func (c BootConfig) Validate() error {
	if _, err := masking.New(c.ToConfig()); err != nil {
		return errors.Wrap(err, "%s", SectionKey)
	}
	return nil
}

// ConfigSection 返回 masking 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &BootConfig{}, func(c *BootConfig, _ *config.Config) error {
		return c.Validate()
	})
}
