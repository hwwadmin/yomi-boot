package masking

import (
	"sort"
	"strings"
	"sync"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// Strategy 脱敏策略：对命中规则的字符串值生成脱敏结果。
//
// ok=false 表示值形状不符，由引擎兜底替换为 placeholder（fail-closed）。
// 契约：ok=true 时不得返回原值；须为纯函数且并发安全；空串由引擎先行短路，不会进入策略。
type Strategy func(value string) (masked string, ok bool)

// reservedStrategyNames 引擎内建解析的策略名，不可用于注册
var reservedStrategyNames = map[string]struct{}{
	StrategyRedact:    {},
	StrategyKeepEdges: {},
}

var (
	registryMu sync.RWMutex
	registry   = make(map[string]Strategy)
)

// MustRegisterStrategy 注册自定义脱敏策略，须在 boot.Run 之前完成
// （masking 插件在 Init 阶段构造服务并解析策略名）。
// 名称为空、fn 为 nil、名称为保留名或与已注册策略冲突时 panic。
func MustRegisterStrategy(name string, fn Strategy) {
	name = strings.TrimSpace(name)
	if name == "" {
		panic(errors.New("masking: 策略名称不能为空"))
	}
	if fn == nil {
		panic(errors.Newf("masking: 策略 %s 的处理函数不能为 nil", name))
	}
	if _, reserved := reservedStrategyNames[name]; reserved {
		panic(errors.Newf("masking: 策略名 %s 为保留名", name))
	}

	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registry[name]; exists {
		panic(errors.Newf("masking: 策略名重复: %s", name))
	}
	registry[name] = fn
}

// lookupStrategy 按名称查找已注册策略
func lookupStrategy(name string) (Strategy, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	fn, ok := registry[name]
	return fn, ok
}

// availableStrategyNames 返回全部可用策略名（内建解析 + 已注册），用于错误提示
func availableStrategyNames() []string {
	registryMu.RLock()
	names := make([]string, 0, len(registry)+len(reservedStrategyNames))
	for name := range registry {
		names = append(names, name)
	}
	registryMu.RUnlock()
	for name := range reservedStrategyNames {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
