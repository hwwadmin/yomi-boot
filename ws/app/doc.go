// Package app 提供基于 ws 原语的高级会话门面：业务收发与连接启停，不直接管理
// Conn / Serve / Hub；底层能力经逃生舱按需使用。
//
// # 与 ws 的关系
//
// 本包只组装 gitee.com/hq_project/yomi-boot/ws，不修改其行为。
// 传输细节（握手、读循环、分片 WriteReader）仍由 ws 承担。
//
// # ConnID（仅服务端）
//
// ConnID 是框架会话句柄，由服务端在连接建立时自动生成并登记 Hub。
// 客户端不感知 ConnID；业务鉴权/身份绑定应在连通后通过业务报文完成，不属本包。
//
// # Client
//
// 一个 Client 实例对应至多一条连接；Connect 自动 Dial 并 Serve，
// 再次 Connect 会先关闭旧连接再建立新连接。
//
// @author zwei
package app
