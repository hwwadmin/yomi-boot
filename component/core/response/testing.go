package response

import "testing"

// RegisterForTest 在测试中覆盖全局 Encoder，并在 t.Cleanup 时恢复原注册状态。
// 允许测试重复覆盖，不受 MustRegister「仅一次」限制。
func RegisterForTest(t testing.TB, enc Encoder) {
	t.Helper()
	registerMu.Lock()
	oldEnc, oldReady := globalEnc, ready
	globalEnc, ready = enc, true
	registerMu.Unlock()
	t.Cleanup(func() {
		registerMu.Lock()
		globalEnc, ready = oldEnc, oldReady
		registerMu.Unlock()
	})
}

// RegisterDefaultForTest 在测试中注册 DefaultEncoder（等同 RegisterForTest(t, DefaultEncoder{})）。
func RegisterDefaultForTest(t testing.TB) {
	t.Helper()
	RegisterForTest(t, DefaultEncoder{})
}
