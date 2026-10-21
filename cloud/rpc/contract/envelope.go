// @author zwei

package contract

import (
	"bytes"
	"context"
	"encoding/json"

	"gitee.com/hq_project/yomi-boot/boot/errors"
	"gitee.com/hq_project/yomi-boot/cloud/rpc"
)

// Envelope 微服务间读响应信封
// Data 保留原始字节，避免嵌套结构的大整数精度丢失
type Envelope struct {
	Code  string          `json:"code"`  // Code 是业务码，OK 表示成功
	Msg   string          `json:"msg"`   // Msg 是业务消息
	Data  json.RawMessage `json:"data"`  // Data 是业务数据
	Total int64           `json:"total"` // Total 是分页总条数，非分页接口为 0
}

// Err 返回业务错误；Code 为 OK 时返回 nil
// Code 为空说明下游未按信封约定返回，返回 ErrDownstream
func (e *Envelope) Err() error {
	if e == nil {
		return errors.New("rpc: 响应信封为空")
	}
	if e.Code == "" {
		return errors.Sentinel(rpc.ErrDownstream)
	}
	if e.Code != "OK" {
		return errors.NewBiz(e.Code, e.Msg)
	}
	return nil
}

// Decode 将 Data 解码到 v；空 data 或 null 时不做处理
func (e *Envelope) Decode(v any) error {
	if e == nil {
		return errors.New("rpc: 响应信封为空")
	}
	if v == nil || len(e.Data) == 0 {
		return nil
	}
	trimmed := bytes.TrimSpace(e.Data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	if err := json.Unmarshal(e.Data, v); err != nil {
		return errors.Wrap(err, "解析响应 data 失败")
	}
	return nil
}

// OK 返回业务是否成功；失败时携带具体错误
func (e *Envelope) OK() (bool, error) {
	if err := e.Err(); err != nil {
		return false, err
	}
	return true, nil
}

// CheckEnvelope 作为 rpc.BizHookFunc 使用，校验信封业务码
func CheckEnvelope(_ context.Context, call rpc.BizHookContext) (bool, error) {
	env, ok := call.RespBody.(*Envelope)
	if !ok {
		return false, errors.New("rpc: 响应类型不是 *contract.Envelope")
	}
	return env.OK()
}
