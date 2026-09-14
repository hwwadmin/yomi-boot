// Package golua Go 与 Lua 值转换
//
// @author zwei
package golua

import (
	"fmt"

	"gitee.com/hq_project/yomi-boot/errors"

	lua "github.com/yuin/gopher-lua"
)

// toLuaValue 将 Go 值转为 Lua 值
func toLuaValue(L *lua.LState, v any, cfg Config, depth int) (lua.LValue, error) {
	if depth > cfg.MaxTableDepth {
		return nil, errors.Sentinel(ErrSandbox)
	}
	if v == nil {
		return lua.LNil, nil
	}
	switch x := v.(type) {
	case bool:
		return lua.LBool(x), nil
	case string:
		if len(x) > cfg.MaxStringLen {
			return nil, errors.Sentinel(ErrSandbox)
		}
		return lua.LString(x), nil
	case int:
		return lua.LNumber(x), nil
	case int8:
		return lua.LNumber(x), nil
	case int16:
		return lua.LNumber(x), nil
	case int32:
		return lua.LNumber(x), nil
	case int64:
		return lua.LNumber(x), nil
	case uint:
		return lua.LNumber(x), nil
	case uint8:
		return lua.LNumber(x), nil
	case uint16:
		return lua.LNumber(x), nil
	case uint32:
		return lua.LNumber(x), nil
	case uint64:
		return lua.LNumber(x), nil
	case float32:
		return lua.LNumber(x), nil
	case float64:
		return lua.LNumber(x), nil
	case map[string]any:
		tbl := L.NewTable()
		if len(x) > cfg.MaxTableSize {
			return nil, errors.Sentinel(ErrSandbox)
		}
		for k, val := range x {
			lv, err := toLuaValue(L, val, cfg, depth+1)
			if err != nil {
				return nil, err
			}
			tbl.RawSetString(k, lv)
		}
		return tbl, nil
	case []any:
		tbl := L.NewTable()
		if len(x) > cfg.MaxTableSize {
			return nil, errors.Sentinel(ErrSandbox)
		}
		for i, val := range x {
			lv, err := toLuaValue(L, val, cfg, depth+1)
			if err != nil {
				return nil, err
			}
			tbl.RawSetInt(i+1, lv)
		}
		return tbl, nil
	default:
		return nil, errors.Newf("golua: 不支持的参数类型 %T", v)
	}
}

// fromLuaValue 将 Lua 值转为 Go 值
func fromLuaValue(v lua.LValue, cfg Config, depth int) (any, error) {
	if depth > cfg.MaxTableDepth {
		return nil, errors.Sentinel(ErrSandbox)
	}
	switch x := v.(type) {
	case *lua.LNilType, nil:
		return nil, nil
	case lua.LBool:
		return bool(x), nil
	case lua.LNumber:
		return float64(x), nil
	case lua.LString:
		s := string(x)
		if len(s) > cfg.MaxStringLen {
			return nil, errors.Sentinel(ErrSandbox)
		}
		return s, nil
	case *lua.LTable:
		return tableToGo(x, cfg, depth)
	default:
		return nil, errors.Newf("golua: 不支持的返回类型 %s", v.Type().String())
	}
}

// tableToGo 将 Lua table 转为 Go slice 或 map
func tableToGo(tbl *lua.LTable, cfg Config, depth int) (any, error) {
	if tbl == nil {
		return nil, nil
	}
	maxN := tbl.MaxN()
	if maxN > 0 {
		// 数组风格
		out := make([]any, 0, maxN)
		for i := 1; i <= maxN; i++ {
			if len(out) >= cfg.MaxTableSize {
				return nil, errors.Sentinel(ErrSandbox)
			}
			lv := tbl.RawGetInt(i)
			gv, err := fromLuaValue(lv, cfg, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, gv)
		}
		return out, nil
	}
	out := make(map[string]any)
	var walkErr error
	count := 0
	tbl.ForEach(func(k, v lua.LValue) {
		if walkErr != nil {
			return
		}
		count++
		if count > cfg.MaxTableSize {
			walkErr = errors.Sentinel(ErrSandbox)
			return
		}
		key := luaValueKey(k)
		gv, err := fromLuaValue(v, cfg, depth+1)
		if err != nil {
			walkErr = err
			return
		}
		out[key] = gv
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return out, nil
}

// luaValueKey 将 Lua 键转为字符串
func luaValueKey(k lua.LValue) string {
	switch x := k.(type) {
	case lua.LString:
		return string(x)
	case lua.LNumber:
		return fmt.Sprintf("%v", float64(x))
	default:
		return k.String()
	}
}
