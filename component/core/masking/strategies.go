package masking

import "strings"

// builtinStrategies 内置脱敏策略（注册表预置条目）；形状不符时返回 ok=false，
// 由引擎降级为 placeholder；所有策略只处理非空字符串（空串由引擎先行短路）。
var builtinStrategies = map[string]Strategy{
	StrategyPhone:    maskPhone,
	StrategyIDCard:   maskIDCard,
	StrategyBankCard: maskBankCard,
	StrategyEmail:    maskEmail,
	StrategyName:     maskName,
}

func init() {
	for name, fn := range builtinStrategies {
		MustRegisterStrategy(name, fn)
	}
}

// maskPhone 大陆手机号：前 3 后 4，如 138****5678；形状要求 11 位数字
func maskPhone(value string) (string, bool) {
	if len(value) != 11 || !isAllDigits(value) {
		return "", false
	}
	return value[:3] + "****" + value[7:], true
}

// maskIDCard 身份证号：前 6 后 4；形状要求 18 位（前 17 位数字 + 数字或 X/x）
func maskIDCard(value string) (string, bool) {
	if len(value) != 18 || !isAllDigits(value[:17]) {
		return "", false
	}
	last := value[17]
	if !isDigit(last) && last != 'X' && last != 'x' {
		return "", false
	}
	return value[:6] + strings.Repeat("*", 8) + value[14:], true
}

// maskBankCard 银行卡号：仅保留后 4 位；形状要求 8~19 位数字
func maskBankCard(value string) (string, bool) {
	if len(value) < 8 || len(value) > 19 || !isAllDigits(value) {
		return "", false
	}
	return strings.Repeat("*", len(value)-4) + value[len(value)-4:], true
}

// maskEmail 邮箱：本地部分保留首字符 + ***，域名保留；本地部分仅 1 字符时整段替换
func maskEmail(value string) (string, bool) {
	at := strings.Index(value, "@")
	if at <= 0 || at == len(value)-1 {
		return "", false
	}
	local, domain := value[:at], value[at:]
	if strings.Contains(domain[1:], "@") {
		return "", false
	}
	localRunes := []rune(local)
	if len(localRunes) <= 1 {
		return "***" + domain, true
	}
	return string(localRunes[0]) + "***" + domain, true
}

// maskName 姓名：保留首字符 + 定长 **（不泄露长度；rune 计数，中文安全）
func maskName(value string) (string, bool) {
	runes := []rune(value)
	if len(runes) == 0 {
		return "", false
	}
	return string(runes[0]) + "**", true
}

// isAllDigits 判断非空字符串是否全为 ASCII 数字
func isAllDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	return true
}

// isDigit 判断字节是否为 ASCII 数字
func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}
