// Package golua 基于 gopher-lua 提供受信业务 Lua 脚本执行能力；按需构造，不进 bootstrap
//
// # 用法
//
//	comp := golua.MustNew(ctx, golua.Config{CacheBackend: golua.CacheMemory}, adapter)
//	comp = golua.MustNew(ctx, golua.Config{
//		CacheBackend:    golua.CacheRedis,
//		Redis:           rdb,
//		DistLockFactory: distlock.MustNewFactory(rdb),
//		RedisKeyPrefix:  "yomi:golua",
//	}, adapter)
//	defer comp.Close()
//	_ = comp.StoreLoad.RefreshScriptForce(ctx, id)
//	out, err := comp.Engine.Execute(ctx, id, args)
//
// # 组件
//
//   - StoreLoadAdapter：业务提供脚本原文
//   - StoreLoad：回源并写入快照缓存
//   - StoreQuery：按 ID 只读快照
//   - Engine：读快照、编译并沙箱执行
//
// # 缓存与热更新
//
// CacheBackend 须显式指定 memory 或 redis。
// Refresh 成功后本机丢弃相关 Proto 与空闲 LState；进行中的 Execute 不中断，归还时按世代丢弃。
// Redis 模式下各进程通过 current_gen 与源码哈希对齐本地编译缓存，任意节点 Refresh 即可。
//
// # 沙箱
//
// 默认拒绝 os/io/package/debug；每请求独立 env；共享全局只读；以 ExecTimeout 为编译与执行硬超时。
//
// @author zwei
package golua
