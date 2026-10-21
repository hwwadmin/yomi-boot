// Package dto 提供跨服务通用数据传输对象
//
// @author zwei
package dto

// MsRequestEnvelope 微服务间请求信封字段（与各接口业务字段同级平铺）
type MsRequestEnvelope struct {
	BizID    string `json:"bizId,omitempty"`    // BizID 是业务幂等 ID
	BizSubID string `json:"bizSubId,omitempty"` // BizSubID 是业务子幂等 ID
}

// MsWriteResponseEnvelope 微服务间写响应回显字段
type MsWriteResponseEnvelope struct {
	BizID    string `json:"bizId,omitempty"`    // BizID 是业务幂等 ID
	BizSubID string `json:"bizSubId,omitempty"` // BizSubID 是业务子幂等 ID
}
