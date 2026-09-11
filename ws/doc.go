// Package ws 提供独立端口 WebSocket：业务只处理会话与报文，不直接碰协议库或读循环。
//
// # 用法
//
//	srv, _ := ws.NewServer(ws.ServerConfig{Addr: ":8080", Processor: myBiz{}})
//
// # Handler / Processor
//
//   - Processor：消费 OnPayload
//   - Handler：启停 + OnPayload；nil 时用 Default*Handler（打日志并调用 Processor）
//   - Config.Name / Processor 仅在 Handler 为 nil 时生效；自定义 Handler 可嵌入 Default*Handler
//   - Send/Broadcast 成功表示已入队（MsgText / MsgBinary）
//
// # 发送
//
// 每连接内存队列 + 单写循环；Ping 可插队，但不能打断进行中的 Write。
// SendQueueSize：0=无界；>0=有界（满则阻塞）。关闭或重连拆旧时丢弃未写出队列。
// Server 已关 → ErrClosed；Client 未连/已关 → ErrNotConnected。
// Broadcast 并行入队，已拆连接跳过；有界满队时调用方仍可能被慢连接堵住。
// 通道健康只看心跳；写出卡住不因此拆线。
// QueueGCInterval：nil→30s；显式 0→关闭。Close 会停巡检，丢弃实例前应 Close。
//
// # 其它
//
// 小文件可走 WS（受 WriteMaxPayloadSize 限制，默认 256KiB，上限 8MiB）；
// 大文件走 filestorage，WS 只做信令。ConnID 仅服务端生成。
// 心跳 / 寿命 / MaxConnections / Origin：nil 与显式 0 语义见各 Config 字段注释。
// Client：单连接；Connect 会先关旧再拨；Close 为实例释放点。
// 无 yml、无默认插件；勿直接依赖 github.com/coder/websocket。
//
// @author zwei
package ws
