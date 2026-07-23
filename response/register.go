// Package response 提供 Encoder 的显式全局注册
//
// @author zwei
package response

import "sync"

var (
	registerMu sync.RWMutex
	globalEnc  Encoder
	ready      bool
)

// MustRegister 在业务 main 中显式注册 Encoder，进程内仅允许调用一次。
// enc 为 nil 或重复注册时 panic。禁止在包 init 中调用本函数。
func MustRegister(enc Encoder) {
	if enc == nil {
		panic("response: Encoder 不能为 nil")
	}
	registerMu.Lock()
	defer registerMu.Unlock()
	if ready {
		panic("response: Encoder 已注册，禁止重复注册")
	}
	globalEnc = enc
	ready = true
}

// MustReady 断言 Encoder 已完成注册。
// 建议在 MustBuildServer 等启动路径调用，使未注册在启动期失败而非首个请求时 panic。
func MustReady() {
	registerMu.RLock()
	defer registerMu.RUnlock()
	if !ready {
		panic("response: 未调用 response.MustRegister，请在 main 中显式注册 Encoder")
	}
}

// Enc 返回已注册的全局 Encoder。
// 未调用 MustRegister 时 panic。
func Enc() Encoder {
	registerMu.RLock()
	defer registerMu.RUnlock()
	if !ready || globalEnc == nil {
		panic("response: 未调用 response.MustRegister，请在 main 中显式注册 Encoder")
	}
	return globalEnc
}

// Registered 返回当前进程是否已完成 Encoder 注册，供诊断或观测旁路判断。
func Registered() bool {
	registerMu.RLock()
	defer registerMu.RUnlock()
	return ready
}

// resetRegisterForTest 清空全局注册状态，仅供本包测试使用。
func resetRegisterForTest() {
	registerMu.Lock()
	defer registerMu.Unlock()
	globalEnc = nil
	ready = false
}
