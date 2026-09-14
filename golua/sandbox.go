// Package golua 沙箱构建与每请求执行环境
//
// @author zwei
package golua

import (
	"strings"

	"gitee.com/hq_project/yomi-boot/errors"

	lua "github.com/yuin/gopher-lua"
)

// newSandboxedState 创建已应用库白名单、base 剔除与全局只读冻结的 LState
func newSandboxedState(cfg Config) (*lua.LState, error) {
	L := lua.NewState(lua.Options{
		SkipOpenLibs:        true,
		CallStackSize:       cfg.CallStackSize,
		RegistrySize:        cfg.RegistrySize,
		IncludeGoStackTrace: false,
	})
	if err := openAllowedLibs(L, cfg.AllowLibs); err != nil {
		L.Close()
		return nil, err
	}
	denyBaseFuncs(L, cfg.DenyBaseFuncs)
	freezeSharedGlobals(L)
	return L, nil
}

// openAllowedLibs 按白名单打开标准库
func openAllowedLibs(L *lua.LState, libs []string) error {
	for _, lib := range libs {
		name := strings.ToLower(strings.TrimSpace(lib))
		switch name {
		case "base":
			lua.OpenBase(L)
		case "table":
			lua.OpenTable(L)
		case "string":
			lua.OpenString(L)
		case "math":
			lua.OpenMath(L)
		case "coroutine":
			lua.OpenCoroutine(L)
		case "utf8":
			// gopher-lua 无独立 utf8 库时跳过
			if err := tryOpenUTF8(L); err != nil {
				return err
			}
		default:
			return errors.Newf("golua: 不支持的 allow_libs: %s", lib)
		}
	}
	return nil
}

// tryOpenUTF8 尝试打开 utf8；当前上游版本无此库时忽略
func tryOpenUTF8(L *lua.LState) error {
	_ = L
	return nil
}

// denyBaseFuncs 从全局表剔除危险 base 函数
func denyBaseFuncs(L *lua.LState, names []string) {
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		L.SetGlobal(name, lua.LNil)
	}
}

// freezeSharedGlobals 禁止向池内共享 _G 写入，防止跨请求污染
func freezeSharedGlobals(L *lua.LState) {
	g, ok := L.Get(lua.GlobalsIndex).(*lua.LTable)
	if !ok || g == nil {
		return
	}
	mt := L.NewTable()
	mt.RawSetString("__newindex", L.NewFunction(func(L *lua.LState) int {
		L.RaiseError("golua: 禁止写入共享全局表")
		return 0
	}))
	mt.RawSetString("__metatable", lua.LString("locked"))
	L.SetMetatable(g, mt)
}

// newExecEnv 创建每请求隔离环境：读回落共享 _G，写只落在 env；env._G 指向自身
func newExecEnv(L *lua.LState) *lua.LTable {
	env := L.NewTable()
	mt := L.NewTable()
	mt.RawSetString("__index", L.Get(lua.GlobalsIndex))
	mt.RawSetString("__metatable", lua.LString("locked"))
	L.SetMetatable(env, mt)
	// 使 _G.x = v 写入 env，而非共享全局
	env.RawSetString("_G", env)
	return env
}
