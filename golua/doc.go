// Package golua 基于 gopher-lua 提供受信业务 Lua 脚本执行能力（按需构造，不进 bootstrap）。
//
// # 用法
//
//	comp := golua.MustNew(ctx, golua.Config{CacheBackend: golua.CacheMemory}, adapter)
//	// 或多实例 Redis：
//	comp = golua.MustNew(ctx, golua.Config{
//		CacheBackend:    golua.CacheRedis,
//		Redis:           rdb,
//		DistLockFactory: distlock.MustNewFactory(rdb),
//		RedisKeyPrefix:  "yomi:golua",
//	}, adapter)
//	defer comp.Close()
//	_ = comp.StoreLoad.RefreshScriptForce(ctx, id) // 业务数据变更后
//	out, err := comp.Engine.Execute(ctx, id, args)
//
// # 职责拆分
//
//   - StoreLoadAdapter：业务实现，全量/按 ID 拉取脚本原文，不碰缓存
//   - 脚本快照缓存：由 Config.CacheBackend 在库内装配（memory / redis+distlock）
//   - StoreLoad：本包实现，回源写入快照缓存并触发本机失效
//   - StoreQuery：本包只读视图，供业务按需直读快照（Engine 不经过此接口）
//   - Engine：直读快照缓存（含 gen）后沙箱执行；Proto 键为源码 content hash
//
// # 热更新与池
//
// 强制刷新不打断进行中的 Execute；写缓存成功后立即丢弃本机空闲旧 Proto / 旧 LState；
// inflight 归还时丢弃，禁止回池复用。
// Redis 后端下其它进程在后续 Execute 中通过 current_gen 水位线自动失效本地编译缓存与池，
// 并以源码哈希作为 Proto 键，业务无需逐副本广播刷新。
//
// # 沙箱
//
// 默认拒绝 os/io/package/debug；每请求独立 env（env._G=env），共享全局只读冻结；
// 硬超时以 Config.ExecTimeout 为准。
//
// @author zwei
package golua
