// 内置插件清单单测。

package registry

import "testing"

// TestPluginsUniqueNames 验证内置插件名称唯一，避免 Runtime.Started 误判
func TestPluginsUniqueNames(t *testing.T) {
	seen := make(map[string]struct{})
	for _, p := range Plugins() {
		if _, ok := seen[p.Name()]; ok {
			t.Fatalf("插件名称重复: %s", p.Name())
		}
		seen[p.Name()] = struct{}{}
	}
}
