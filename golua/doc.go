// Package golua 基于 gopher-lua 提供受信业务 Lua 脚本执行能力（按需构造，不进 bootstrap）。
//
// # 用法
//
//	cache := golua.NewMemoryScriptCache()
//	// 或多实例：
//	factory := distlock.MustNewFactory(rdb)
//	cache, _ := golua.NewRedisScriptCache(rdb, "yomi:golua", factory)
//	comp := golua.MustNew(ctx, golua.Config{}, adapter, cache)
//	defer comp.Close()
//	_ = comp.StoreLoad.RefreshScriptForce(ctx, id) // 业务数据变更后
//	out, err := comp.Engine.Execute(ctx, id, args)
//
// # 职责拆分
//
//   - StoreLoadAdapter：业务实现，全量/按 ID 拉取脚本原文，不碰缓存
//   - ScriptCache：内存或 Redis，由业务注入
//   - StoreLoad / StoreQuery：本包实现，读写 ScriptCache
//   - Engine：只经 StoreQuery 读缓存后沙箱执行
//
// # 热更新与池
//
// 强制刷新不打断进行中的 Execute；写缓存成功后立即丢弃空闲旧 Proto / 旧 LState；
// inflight 归还时丢弃，禁止回池复用。
//
// # 沙箱
//
// 默认拒绝 os/io/package/debug；每请求独立 env（env._G=env），共享全局只读冻结；
// 硬超时以 Config.ExecTimeout 为准。
//
// @author zwei
package golua
