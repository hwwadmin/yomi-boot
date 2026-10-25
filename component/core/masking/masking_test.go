// 统一脱敏服务的单元测试
//
// @author zwei
package masking

import (
	"net/url"
	"strings"
	"testing"
)

// newTestService 按通用规则构建测试服务
func newTestService(t *testing.T, cfg Config) *Service {
	t.Helper()
	svc, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return svc
}

// TestNewValidate 验证配置校验的错误形态
func TestNewValidate(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{
			name: "非法 match", cfg: Config{Match: "regex", Rules: []RuleConfig{{Keys: []string{"password"}, Strategy: StrategyRedact}}},
			want: "match 仅支持",
		},
		{name: "策略为空", cfg: Config{Rules: []RuleConfig{{Keys: []string{"password"}}}}, want: "strategy 不能为空"},
		{name: "keys 为空", cfg: Config{Rules: []RuleConfig{{Keys: []string{"  "}, Strategy: StrategyRedact}}}, want: "keys 不能为空"},
		{
			name: "keep 参数为负", cfg: Config{Rules: []RuleConfig{{Keys: []string{"order"}, Strategy: StrategyKeepEdges, KeepHead: -1}}},
			want: "不能为负",
		},
		{
			name: "非 keep_edges 配 keep 参数", cfg: Config{Rules: []RuleConfig{{Keys: []string{"order"}, Strategy: StrategyPhone, KeepHead: 3}}},
			want: "仅 keep_edges",
		},
		{
			name: "keep_edges 零参数", cfg: Config{Rules: []RuleConfig{{Keys: []string{"order"}, Strategy: StrategyKeepEdges}}},
			want: "keep_head+keep_tail 须 > 0",
		},
		{
			name: "未注册策略", cfg: Config{Rules: []RuleConfig{{Keys: []string{"order"}, Strategy: "no_such_strategy"}}},
			want: "未注册的策略",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("期望错误含 %q，实际 %v", tc.want, err)
			}
		})
	}
}

// TestMatchContainsAndExact 验证 contains / exact 匹配与首条命中优先
func TestMatchContainsAndExact(t *testing.T) {
	contains := newTestService(t, Config{Rules: []RuleConfig{
		{Keys: []string{"token", "password"}, Strategy: StrategyRedact},
	}})
	if !contains.ShouldMask("access_token") || !contains.ShouldMask("  PASSWORD ") {
		t.Fatal("contains 应命中且大小写/空白不敏感")
	}
	if contains.ShouldMask("username") {
		t.Fatal("username 不应命中")
	}

	exact := newTestService(t, Config{Match: MatchExact, Rules: []RuleConfig{
		{Keys: []string{"token"}, Strategy: StrategyRedact},
	}})
	if exact.ShouldMask("access_token") {
		t.Fatal("exact 不应命中 access_token")
	}
	if !exact.ShouldMask("token") {
		t.Fatal("exact 应命中 token")
	}

	first := newTestService(t, Config{Rules: []RuleConfig{
		{Keys: []string{"card"}, Strategy: StrategyRedact},
		{Keys: []string{"bank_card"}, Strategy: StrategyBankCard},
	}})
	if got := first.Mask("bank_card_no", "6222020200112233445"); got != DefaultPlaceholder {
		t.Fatalf("首条命中应生效，实际 %q", got)
	}
}

// TestBuiltinStrategies 验证内置策略的形状矩阵
func TestBuiltinStrategies(t *testing.T) {
	svc := newTestService(t, Config{Rules: []RuleConfig{
		{Keys: []string{"phone"}, Strategy: StrategyPhone},
		{Keys: []string{"idcard"}, Strategy: StrategyIDCard},
		{Keys: []string{"bankcard"}, Strategy: StrategyBankCard},
		{Keys: []string{"email"}, Strategy: StrategyEmail},
		{Keys: []string{"name"}, Strategy: StrategyName},
	}})

	cases := []struct {
		field, in, want string
	}{
		{"phone", "13812345678", "138****5678"},
		{"phone", "1381234567", DefaultPlaceholder},   // 10 位
		{"phone", "13812345678x", DefaultPlaceholder}, // 非数字
		{"idcard", "11010119900307751X", "110101********751X"},
		{"idcard", "11010119900307751x", "110101********751x"},
		{"idcard", "1101011990030775", DefaultPlaceholder},   // 长度不足
		{"idcard", "1101011990030775ZZ", DefaultPlaceholder}, // 尾号非法
		{"bankcard", "6222020200112233445", "***************3445"},
		{"bankcard", "1234", DefaultPlaceholder}, // 太短
		{"email", "zwei@example.com", "z***@example.com"},
		{"email", "a@x.com", "***@x.com"},
		{"email", "user@@x.com", DefaultPlaceholder}, // 多个 @
		{"email", "@x.com", DefaultPlaceholder},      // 本地部分为空
		{"name", "张三丰", "张**"},
		{"name", "John", "J**"},
		{"phone", "", ""}, // 空串短路
	}
	for _, tc := range cases {
		if got := svc.Mask(tc.field, tc.in); got != tc.want {
			t.Fatalf("Mask(%q, %q) 期望 %q，实际 %q", tc.field, tc.in, tc.want, got)
		}
	}
}

// TestKeepEdges 验证 keep_edges 的参数应用、越界降级与 rune 安全
func TestKeepEdges(t *testing.T) {
	svc := newTestService(t, Config{Rules: []RuleConfig{
		{Keys: []string{"order_no"}, Strategy: StrategyKeepEdges, KeepHead: 3, KeepTail: 4},
	}})
	if got := svc.Mask("order_no", "AB2026000123"); got != "AB2*****0123" {
		t.Fatalf("keep_edges 结果不符: %q", got)
	}
	if got := svc.Mask("order_no", "你好世界2026"); got != "你好世*2026" {
		t.Fatalf("keep_edges 应按 rune 计数: %q", got)
	}
	if got := svc.Mask("order_no", "AB2026"); got != DefaultPlaceholder {
		t.Fatalf("head+tail 不小于长度应降级 placeholder: %q", got)
	}
}

// TestCustomStrategy 验证自定义策略注册与 fail-closed 降级
func TestCustomStrategy(t *testing.T) {
	MustRegisterStrategy("test_plate", func(v string) (string, bool) {
		runes := []rune(v)
		if len(runes) != 7 {
			return "", false
		}
		return string(runes[:2]) + "***" + string(runes[5:]), true
	})
	svc := newTestService(t, Config{Rules: []RuleConfig{
		{Keys: []string{"plate_no"}, Strategy: "test_plate"},
	}})
	if got := svc.Mask("plate_no", "京A12345"); got != "京A***45" {
		t.Fatalf("自定义策略结果不符: %q", got)
	}
	if got := svc.Mask("plate_no", "京A-123"); got != DefaultPlaceholder {
		t.Fatalf("形状不符应降级 placeholder: %q", got)
	}

	// 违约返回原值时由引擎兜底
	MustRegisterStrategy("test_echo", func(v string) (string, bool) { return v, true })
	echo := newTestService(t, Config{Rules: []RuleConfig{
		{Keys: []string{"k"}, Strategy: "test_echo"},
	}})
	if got := echo.Mask("k", "raw"); got != DefaultPlaceholder {
		t.Fatalf("策略返回原值应由引擎兜底: %q", got)
	}
}

// TestRegistryListing 验证注册表清单查询：内置与自定义的区分
func TestRegistryListing(t *testing.T) {
	for _, name := range []string{StrategyPhone, StrategyIDCard, StrategyBankCard, StrategyEmail, StrategyName} {
		if !contains(RegisteredStrategies(), name) {
			t.Fatalf("RegisteredStrategies 应含内置策略 %s", name)
		}
	}
	if contains(CustomStrategyNames(), StrategyPhone) {
		t.Fatal("CustomStrategyNames 不应含内置策略")
	}
	// TestCustomStrategy 已注册 test_plate / test_echo
	for _, name := range []string{"test_plate", "test_echo"} {
		if !contains(RegisteredStrategies(), name) || !contains(CustomStrategyNames(), name) {
			t.Fatalf("注册清单应含自定义策略 %s", name)
		}
	}
	if contains(RegisteredStrategies(), StrategyRedact) {
		t.Fatal("RegisteredStrategies 不应含引擎内建解析的保留名")
	}
}

// contains 判断切片是否含目标元素
func contains(names []string, target string) bool {
	for _, name := range names {
		if name == target {
			return true
		}
	}
	return false
}

// TestMustRegisterStrategyPanics 验证注册表的非法入参
func TestMustRegisterStrategyPanics(t *testing.T) {
	cases := []struct {
		name string
		call func()
	}{
		{"空名称", func() { MustRegisterStrategy("", maskPhone) }},
		{"nil 函数", func() { MustRegisterStrategy("test_nil", nil) }},
		{"保留名 redact", func() { MustRegisterStrategy(StrategyRedact, maskPhone) }},
		{"保留名 keep_edges", func() { MustRegisterStrategy(StrategyKeepEdges, maskPhone) }},
		{"覆盖内置策略", func() { MustRegisterStrategy(StrategyPhone, maskPhone) }},
		{"重复注册", func() {
			MustRegisterStrategy("test_dup", maskEmail)
			MustRegisterStrategy("test_dup", maskEmail)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("期望 panic")
				}
			}()
			tc.call()
		})
	}
}

// TestMaskJSON 验证 JSON 递归脱敏、大数精度与解析失败透传
func TestMaskJSON(t *testing.T) {
	svc := newTestService(t, Config{Rules: []RuleConfig{
		{Keys: []string{"password", "token"}, Strategy: StrategyRedact},
		{Keys: []string{"phone"}, Strategy: StrategyPhone},
	}})

	raw := `{"name":"u","password":"p@ss","nested":{"token":"tk","keep":1},"list":[{"phone":"13812345678"}],"big":12345678901234567890,"token_num":123456}`
	got := svc.MaskJSON(raw)
	for _, want := range []string{`"password":"***"`, `"token":"***"`, `"phone":"138****5678"`, `"keep":1`, `"big":12345678901234567890`, `"token_num":"***"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("脱敏结果缺少 %s: %s", want, got)
		}
	}
	if strings.Contains(got, "p@ss") || strings.Contains(got, `"tk"`) {
		t.Fatalf("敏感值未脱敏: %s", got)
	}

	if got := svc.MaskJSON("not a json"); got != "not a json" {
		t.Fatalf("非 JSON 应原样返回: %q", got)
	}
	if got := svc.MaskJSON(`{"broken":`); got != `{"broken":` {
		t.Fatalf("解析失败应原样返回: %q", got)
	}
	if got := svc.MaskJSON("   "); got != "   " {
		t.Fatalf("空串应原样返回: %q", got)
	}
}

// TestMaskQueryAndHeaderMap 验证 query 与 header 应用面
func TestMaskQueryAndHeaderMap(t *testing.T) {
	svc := newTestService(t, Config{Rules: []RuleConfig{
		{Keys: []string{"token", "authorization"}, Strategy: StrategyRedact},
		{Keys: []string{"phone"}, Strategy: StrategyPhone},
	}})

	got := svc.MaskQuery("access_token=secret&q=1&phone=13812345678")
	if !strings.Contains(got, "q=1") {
		t.Fatalf("普通参数应保留: %s", got)
	}
	// Encode 会把 * 转义为 %2A，先解码再断言脱敏结果
	decoded, err := url.QueryUnescape(got)
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}
	if strings.Contains(decoded, "secret") || !strings.Contains(decoded, "phone=138****5678") {
		t.Fatalf("敏感参数未按策略脱敏: %s", decoded)
	}
	if got := svc.MaskQuery("%zz=broken"); got != "%zz=broken" {
		t.Fatalf("解析失败应原样返回: %q", got)
	}

	headers := svc.MaskHeaderMap(map[string]string{"authorization": "Bearer x", "content-type": "application/json"})
	if headers["authorization"] != DefaultPlaceholder {
		t.Fatalf("敏感头应脱敏: %v", headers["authorization"])
	}
	if headers["content-type"] != "application/json" {
		t.Fatalf("普通头应保留: %v", headers["content-type"])
	}
}

// TestMaskKV 验证 zap 风格 KV 脱敏与值递归
func TestMaskKV(t *testing.T) {
	svc := newTestService(t, Config{Rules: []RuleConfig{
		{Keys: []string{"password", "token"}, Strategy: StrategyRedact},
	}})

	kv := svc.MaskKV("password", "p@ss", "user", "a", "count", 3)
	if kv[1] != DefaultPlaceholder || kv[3] != "a" || kv[5] != 3 {
		t.Fatalf("KV 脱敏不符: %v", kv)
	}

	kv = svc.MaskKV("password", 12345)
	if kv[1] != DefaultPlaceholder {
		t.Fatalf("非字符串命中值应替换为 placeholder: %v", kv[1])
	}

	kv = svc.MaskKV("payload", `{"token":"tk"}`)
	if !strings.Contains(kv[1].(string), `"token":"***"`) {
		t.Fatalf("未命中键的 JSON 字符串值应递归脱敏: %v", kv[1])
	}

	odd := svc.MaskKV("password", "p@ss", "extra")
	if len(odd) != 3 || odd[1] != DefaultPlaceholder || odd[2] != "extra" {
		t.Fatalf("奇数个应仍处理完整对、末尾落单 key 原样: %v", odd)
	}
}

// TestMaskMap 验证 map 递归脱敏与未启用透传
func TestMaskMap(t *testing.T) {
	svc := newTestService(t, Config{Rules: []RuleConfig{
		{Keys: []string{"phone"}, Strategy: StrategyPhone},
	}})
	in := map[string]any{"phone": "13812345678", "nested": map[string]any{"phone": "13987654321"}}
	got := svc.MaskMap(in)
	if got["phone"] != "138****5678" || got["nested"].(map[string]any)["phone"] != "139****4321" {
		t.Fatalf("MaskMap 结果不符: %v", got)
	}
	if _, changed := in["phone"].(string); !changed || in["phone"] != "13812345678" {
		t.Fatal("入参 map 不应被修改")
	}

	noop := Noop()
	if got := noop.MaskMap(in); got == nil || got["phone"] != "13812345678" {
		t.Fatal("Noop 应透传")
	}
}

// TestNilAndNoopSafety 验证 nil 接收者与空规则的透传语义
func TestNilAndNoopSafety(t *testing.T) {
	var nilSvc *Service
	if nilSvc.Active() || nilSvc.ShouldMask("password") {
		t.Fatal("nil 服务应视为未启用")
	}
	if got := nilSvc.Mask("password", "x"); got != "x" {
		t.Fatalf("nil Mask 应透传: %q", got)
	}
	if got := nilSvc.MaskJSON(`{"password":"x"}`); got != `{"password":"x"}` {
		t.Fatalf("nil MaskJSON 应透传: %q", got)
	}
	if got := nilSvc.MaskKV("password", "x"); got[1] != "x" {
		t.Fatal("nil MaskKV 应透传")
	}

	empty := newTestService(t, Config{})
	if empty.Active() || empty.Mask("password", "x") != "x" {
		t.Fatal("空规则服务应透传")
	}
}

// TestDefaultPlaceholder 验证占位符默认值与自定义
func TestDefaultPlaceholder(t *testing.T) {
	def := newTestService(t, Config{Rules: []RuleConfig{
		{Keys: []string{"password"}, Strategy: StrategyRedact},
	}})
	if got := def.Mask("password", "x"); got != DefaultPlaceholder {
		t.Fatalf("默认占位符不符: %q", got)
	}

	custom := newTestService(t, Config{Placeholder: "[REDACTED]", Rules: []RuleConfig{
		{Keys: []string{"password"}, Strategy: StrategyRedact},
	}})
	if got := custom.Mask("password", "x"); got != "[REDACTED]" {
		t.Fatalf("自定义占位符不符: %q", got)
	}
}

// TestSetDefaultAndCurrent 验证全局默认实例注入
func TestSetDefaultAndCurrent(t *testing.T) {
	t.Cleanup(func() { SetDefault(nil) })

	if Current().Active() {
		t.Fatal("未设置时 Current 应透传")
	}
	svc := newTestService(t, Config{Rules: []RuleConfig{
		{Keys: []string{"phone"}, Strategy: StrategyPhone},
	}})
	SetDefault(svc)
	if Current() != svc {
		t.Fatal("SetDefault 后 Current 应返回同一实例")
	}
	if got := Current().Mask("phone", "13812345678"); got != "138****5678" {
		t.Fatalf("Current 脱敏不符: %q", got)
	}
	SetDefault(nil)
	if Current().Active() {
		t.Fatal("SetDefault(nil) 后应透传")
	}
}
