// Package wsbase 提供基于 gws 的 WebSocket 传输原语：独立端口 Server、可 Dial 的 Client、
// 共用 Conn/Handler；一条逻辑消息对应一次 OnMessage（gws 负责帧级合帧）。
//
// # 定位
//
// 本包是传输层能力库：业务或上层门面自行准备 ServerConfig / ClientConfig 并多次 New*
// 启动多个实例；不提供 yml 配置段，也不挂默认 infra 插件。
//
// # 主路径
//
//   - Server：独立 TCP 端口监听并完成 WebSocket 握手，与业务 HTTP/gin 解耦
//   - Client：Dial 出站连接；业务需 go Conn.Serve() 启动读循环
//   - 连接读写统一走 Conn；缺口能力经 Native() 取 *gws.Conn
//
// # 载荷大小
//
// WriteText / WriteBinary 写入一条逻辑消息。WriteMaxPayloadSize 限制单条可写上限
// （映射 gws，默认 256KiB）；超过则写入失败。ReadMaxPayloadSize 限制单条可读上限。
// 本包不再提供应用层「一片一消息」的分片发送。
// 小文件可走本通道；大文件应经 filestorage（MinIO / S3 / OSS）间接传输，
// WS 只传控制报文。勿靠调高读写上限把对象整包塞进一条消息。
//
// # 心跳
//
// ServerConfig / ClientConfig 均支持 HeartbeatInterval 与 HeartbeatTimeout（*time.Duration），
// 语义与 MaxConnLifetime 一致：nil→该端默认常量；显式 0→关闭对应能力：
//
//   - Server Interval：nil→DefaultServerHeartbeatInterval（0，不主动发 Ping）
//   - Client Interval：nil→DefaultClientHeartbeatInterval（10s）
//   - Timeout（两端）：nil→DefaultHeartbeatTimeout（30s）；显式 0→不因空闲超时踢连接
//   - Interval 显式 0：不发 Ping
//   - 两者均 >0 时要求 Timeout > Interval
//
// Timeout>0 时本包接管该连接 ReadDeadline；任意入站（消息 / Ping / Pong）会刷新。
// Interval>0 时周期 WritePing；写失败则关闭连接。CustomizeGWS 勿再与心跳争用 ReadDeadline。
// 常规用法：客户端发 Ping，服务端只做空闲超时（默认即如此）。
//
// # 通道最长存活（仅 Server）
//
// ServerConfig 支持 MaxConnLifetime 与 MaxConnLifetimeJitter（*time.Duration）：
//
//   - nil：分别使用 DefaultMaxConnLifetime（24h）与 DefaultMaxConnLifetimeJitter（30s）
//   - 显式 0：Lifetime 表示不限制存活；Jitter 表示不波动
//   - 两者均 >0 时要求 Jitter < Lifetime
//
// 从握手成功起算墙钟时间，流量不续命。实际断开时刻为基准加减波动的均匀随机，
// 到期后服务端以 CloseGoingAway 主动断开。ClientConfig 无此配置。
//
// # 连接数上限（仅 Server）
//
// ServerConfig.MaxConnections 控制同时在线连接数：
//
//   - 0：不限制（默认）
//   - >0：握手成功后占槽；超过上限则以 CloseTryAgainLater 立即断开
//   - 负数：Validate 失败
//
// # 高级门面
//
// 若希望业务不直接面对 Conn/Serve/Hub，请使用上层包 gitee.com/hq_project/yomi-boot/ws。
// 其中 ConnID 仅服务端自动生成；业务鉴权应在连通后用报文完成，不属本包。
//
// # 读模型
//
// 同一连接只能使用一种读模型；门面默认 Serve → gws ReadLoop。禁止与 Native 侧
// ReadMessage/NextReader 混用。
//
// @author zwei
package wsbase
