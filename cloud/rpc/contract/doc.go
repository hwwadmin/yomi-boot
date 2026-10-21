// Package contract 定义微服务间读响应信封与业务成功判定。
//
// 传输层只负责 HTTP 200 与反序列化；本包通过 Envelope 解析 code/data/total，
// 并把非 OK 的业务码映射为 BizError。分页调用方直接读取 Envelope.Total。
//
// @author zwei
package contract
