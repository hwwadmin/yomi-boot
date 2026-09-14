// Package golua 基于 gopher-lua 提供受信业务 Lua 脚本执行能力。
//
// # 职责拆分
//
//   - StoreLoadAdapter：业务实现，全量/按 ID 拉取脚本原文，不碰缓存
//   - StoreLoad：本包实现，普通/强制刷新写入 Redis，并失效本地编译缓存与 LState 池世代
//   - StoreQuery：本包实现，仅读 Redis；Engine 只依赖本接口
//
// # 热更新与池
//
// 强制刷新不打断进行中的 Execute；成功写入 Redis 后立即丢弃空闲旧 Proto / 旧 LState；
// inflight 使用的旧对象在归还时丢弃，禁止回池复用。
//
// # 沙箱
//
// 默认拒绝 os/io/package/debug 等库；每请求独立 env（env._G=env），并冻结共享全局只读。
// 执行使用 context 墙钟超时（gopher-lua 在每条 VM 指令处检查）。上游无 sethook 时，
// max_instructions 仅作配置项保留，硬限制以 exec_timeout_ms 为准。
//
// # 启用
//
// golua.enabled 未配置默认 false，不启动。
//
// @author zwei
package golua
