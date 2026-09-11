// Package ws 提供独立端口 WebSocket：业务只面对会话与报文，不直接管理读循环或协议库。
//
// # 用法（推荐）
//
// 业务实现 ServerMessageProcessor / ClientMessageProcessor，写入 Config 后启动：
//
//	srv, _ := ws.NewServer(ws.ServerConfig{
//	    Addr:      ":8080",
//	    Processor: myBiz{},
//	})
//
// # Handler / Processor
//
//   - Processor：只负责 OnPayload 消费业务消息（Payload.Type + Data）
//   - Handler：会话面（启停 + OnPayload）；Config.Handler 为 nil 时用
//     Default*Handler（启停默认打日志，OnPayload 调用 Processor）
//   - 发送统一为 Send/Broadcast(typ, data)，类型用 MsgText / MsgBinary；成功表示已入队
//   - 连通后主动发信由业务在 Connect 成功后自行调用 Client/Server API
//   - 自定义 Handler 时 Config.Processor 不自动接入；若仍需 Processor，
//     可嵌入 Default*Handler 并自行赋值 Processor
//
// # 发送队列
//
// 每连接一条内存发送队列，写循环串行写出；心跳 Ping 相对业务消息插队。
// SendQueueSize：0=无界；>0=有界（满则阻塞入队）。Server 已关闭时 Send/Broadcast
// 立即返回 ErrClosed；Client 未连接或连接已关返回 ErrNotConnected。
// Broadcast 入队时连接已拆则跳过该连接。通道健康只由心跳判定；写出卡住允许存在，
// 不因发送超时拆线。QueueGCInterval：nil→30s 巡检孤儿队列；显式 0→关闭巡检。
//
// # 载荷与文件
//
// OnPayload 在一条逻辑消息收齐后触发，整包进内存。
// 小文件可直接 Send(MsgBinary, data)，体量对齐 WriteMaxPayloadSize
// （默认 256KiB，配置上限 8MiB）。大文件不要走 WebSocket 整包，应经
// filestorage（MinIO / S3 / OSS）上传或预签名直传；WS 只推 uri、进度、
// 完成/失败等控制报文。勿靠调高读写上限把对象塞进一条消息。
//
// # ConnID（仅服务端）
//
// 由服务端在连接建立时自动生成并登记；客户端不感知。
// 业务鉴权应在连通后通过报文完成，不属本包。
//
// # 心跳
//
// HeartbeatInterval、HeartbeatTimeout：nil→该端默认常量；显式 0→关闭对应能力。
// 默认：服务端不主动 Ping + 30s 空闲超时；客户端 10s Ping + 30s 超时。
// Timeout>0 时任意入站（消息 / Ping / Pong）刷新空闲定时器。
//
// # 通道最长存活（仅 Server）
//
// MaxConnLifetime、MaxConnLifetimeJitter：nil 分别为 24h 与 30s；显式 0 表示不限制 / 不波动。
// 到期服务端主动断开，流量不续命。
//
// # 连接数上限（仅 Server）
//
// MaxConnections：0 不限制；>0 为同时在线上限，超额连接握手后以 CloseTryAgainLater 断开。
//
// # Origin
//
// OriginPatterns 为空时不做 Origin 校验（独立端口默认，便于非浏览器客户端）。
// 浏览器跨源场景应显式填写允许的 Origin host 模式。
//
// # Client
//
// 一个 Client 实例对应至多一条连接；Connect 自动 Dial 并启动读循环。
//
// 本包不提供 yml 配置段，也不挂默认 infra 插件；构造期 Config 多实例装配。
// 业务不要直接依赖 github.com/coder/websocket。
//
// @author zwei
package ws
