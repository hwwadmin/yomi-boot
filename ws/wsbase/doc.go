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
