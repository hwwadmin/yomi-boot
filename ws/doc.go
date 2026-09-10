// Package ws 提供基于 wsbase 的高级会话门面：业务收发与连接启停，不直接管理
// Conn / Serve / Hub；底层能力经逃生舱或 wsbase 按需使用。
//
// # 与 wsbase 的关系
//
// 本包只组装 gitee.com/hq_project/yomi-boot/ws/wsbase，不修改其行为。
//
// # 用法（推荐）
//
// 业务实现 MessageProcessor，写入 Config 后启动：
//
//	srv, _ := ws.NewServer(ws.ServerConfig{
//	    ServerConfig: wsbase.ServerConfig{Addr: ":8080"},
//	    Processor:    myBiz{},
//	})
//
// 常用传输类型也可经本包别名使用（见 alias.go），多数场景无需再 import wsbase。
//
// # Handler / Processor
//
//   - Processor：只负责 OnPayload 消费业务消息（主扩展点）
//   - Handler：会话面（启停 + 原始 Message）；Config.Handler 为 nil 时用
//     Default*Handler（启停默认打日志，OnMessage 调用 Processor）
//   - 连通后主动发信由业务在 Connect 成功后自行调用 Client/Server API，
//     不属于 Processor
//   - 自定义 Handler 时 Config.Processor 不自动接入；若仍需 Processor，
//     可嵌入 Default*Handler 并自行赋值 Processor
//
// # ConnID（仅服务端）
//
// 由服务端在连接建立时自动生成并登记 Hub；客户端不感知。
// 业务鉴权应在连通后通过报文完成，不属本包。
//
// # Client
//
// 一个 Client 实例对应至多一条连接；Connect 自动 Dial 并 Serve。
//
// @author zwei
package ws
