package contract

import (
	"context"
	"encoding/json"
	"testing"

	"gitee.com/hq_project/yomi-boot/boot/errors"
	"gitee.com/hq_project/yomi-boot/cloud/rpc"
)

// TestEnvelopeErr 验证业务码到错误的映射
func TestEnvelopeErr(t *testing.T) {
	if err := (&Envelope{Code: "OK"}).Err(); err != nil {
		t.Fatalf("OK 应返回 nil，实际 %v", err)
	}

	err := (&Envelope{Code: "INVALID_INPUT", Msg: "bad"}).Err()
	var biz *errors.BizError
	if !errors.As(err, &biz) || biz.Code != "INVALID_INPUT" {
		t.Fatalf("期望 INVALID_INPUT，实际 %v", err)
	}

	if err := (&Envelope{}).Err(); !errors.Is(err, rpc.ErrDownstream) {
		t.Fatalf("空 code 应返回 ErrDownstream，实际 %v", err)
	}

	var nilEnvelope *Envelope
	if err := nilEnvelope.Err(); err == nil {
		t.Fatal("nil 信封应返回错误")
	}
}

// TestEnvelopeDecode 验证 Data 解码
func TestEnvelopeDecode(t *testing.T) {
	type payload struct {
		ID int64 `json:"id"`
	}

	env := &Envelope{Data: json.RawMessage(`{"id":9223372036854775807}`)}
	var got payload
	if err := env.Decode(&got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.ID != 9223372036854775807 {
		t.Fatalf("id: got %d", got.ID)
	}

	for _, raw := range []string{"", "null", "  "} {
		env := &Envelope{Data: json.RawMessage(raw)}
		var empty payload
		if err := env.Decode(&empty); err != nil {
			t.Fatalf("Data=%q 应忽略，实际 %v", raw, err)
		}
	}

	if err := (&Envelope{Data: json.RawMessage(`not json`)}).Decode(&got); err == nil {
		t.Fatal("非法 JSON 应返回错误")
	}
	if err := (*Envelope)(nil).Decode(&got); err == nil {
		t.Fatal("nil 信封应返回错误")
	}
}

// TestEnvelopeOK 验证业务成功判定
func TestEnvelopeOK(t *testing.T) {
	if ok, err := (&Envelope{Code: "OK"}).OK(); !ok || err != nil {
		t.Fatalf("OK 应返回 true,nil，实际 %v,%v", ok, err)
	}

	ok, err := (&Envelope{Code: "INVALID_INPUT"}).OK()
	var biz *errors.BizError
	if ok || !errors.As(err, &biz) || biz.Code != "INVALID_INPUT" {
		t.Fatalf("期望 false,INVALID_INPUT，实际 %v,%v", ok, err)
	}

	ok, err = (&Envelope{}).OK()
	if ok || !errors.Is(err, rpc.ErrDownstream) {
		t.Fatalf("空 code 应返回 false,ErrDownstream，实际 %v,%v", ok, err)
	}
}

// TestCheckEnvelope 验证 hook 适配
func TestCheckEnvelope(t *testing.T) {
	if ok, err := CheckEnvelope(context.Background(), rpc.BizHookContext{RespBody: &Envelope{Code: "OK"}}); !ok || err != nil {
		t.Fatalf("OK 应返回 true,nil，实际 %v,%v", ok, err)
	}

	ok, err := CheckEnvelope(context.Background(), rpc.BizHookContext{RespBody: &Envelope{Code: "INVALID_INPUT"}})
	var biz *errors.BizError
	if ok || !errors.As(err, &biz) || biz.Code != "INVALID_INPUT" {
		t.Fatalf("期望 false,INVALID_INPUT，实际 %v,%v", ok, err)
	}

	if ok, err := CheckEnvelope(context.Background(), rpc.BizHookContext{RespBody: "wrong"}); ok || err == nil {
		t.Fatal("错误响应类型应返回 false,error")
	}
	if ok, err := CheckEnvelope(context.Background(), rpc.BizHookContext{}); ok || err == nil {
		t.Fatal("nil RespBody 应返回 false,error")
	}
}
