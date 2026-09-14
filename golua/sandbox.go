// Package golua 沙箱与每请求执行环境
//
// @author zwei
package golua

import (
	"strings"

	"gitee.com/hq_project/yomi-boot/errors"

	lua "github.com/yuin/gopher-lua"
)

// newSandboxedState 创建沙箱 LState
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
			if err := tryOpenUTF8(L); err != nil {
				return err
			}
		default:
			return errors.Newf("golua: 不支持的 allow_libs: %s", lib)
		}
	}
	return nil
}

// tryOpenUTF8 打开 utf8；不可用时忽略
func tryOpenUTF8(L *lua.LState) error {
	_ = L
	return nil
}

// denyBaseFuncs 从全局表剔除指定函数
func denyBaseFuncs(L *lua.LState, names []string) {
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		L.SetGlobal(name, lua.LNil)
	}
}

// freezeSharedGlobals 将共享 _G 设为只读
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

// newExecEnv 创建每请求隔离环境；读回落共享 _G，写落在 env
func newExecEnv(L *lua.LState) *lua.LTable {
	env := L.NewTable()
	mt := L.NewTable()
	mt.RawSetString("__index", L.Get(lua.GlobalsIndex))
	mt.RawSetString("__metatable", lua.LString("locked"))
	L.SetMetatable(env, mt)
	env.RawSetString("_G", env)
	return env
}
