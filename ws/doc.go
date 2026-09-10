// Package ws 提供基于 gws 的 WebSocket 能力库：独立端口 Server、可 Dial 的 Client、
// 共用 Conn/Handler，以及可配置分片大小的大负载流式写入。
//
// # 定位
//
// 本包是构造入口型能力库：业务自行准备 ServerConfig / ClientConfig 并多次 New*
// 启动多个实例；不提供 yml 配置段，也不挂默认 infra 插件。
//
// # 主路径
//
//   - Server：独立 TCP 端口监听并完成 WebSocket 握手，与业务 HTTP/gin 解耦
//   - Client：Dial 出站连接；业务需 go Conn.Serve() 启动读循环
//   - 连接读写统一走 Conn；缺口能力经 Native() 取 *gws.Conn
//
// # 大文件
//
// WriteReader 按可配置 chunkSize 循环读取 io.Reader，默认「一片一完整 binary/text 消息」
// （非单消息底层续帧），以便收发两端峰值内存均可控。分片大小优先级：
// WriteOption > 实例 Config.ChunkSize > 库默认（256KiB）。
//
// # 高级门面
//
// 若希望业务不直接面对 Conn/Serve/Hub，请使用子包 gitee.com/hq_project/yomi-boot/ws/app。
// 其中 ConnID 仅服务端自动生成；业务鉴权应在连通后用报文完成，不属 ws 传输层。
//
// # 读模型
//
// 同一连接只能使用一种读模型；门面默认 Serve → gws ReadLoop。禁止与 Native 侧
// ReadMessage/NextReader 混用。
//
// @author zwei
package ws
