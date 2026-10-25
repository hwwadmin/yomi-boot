// DTM 分支处理辅助的单元测试

package dtm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/core/errors"

	"github.com/dtm-labs/client/dtmcli"
	"github.com/gin-gonic/gin"
)

// newGinContext 构造带请求的 Gin 测试上下文，返回上下文与响应记录器
func newGinContext(t *testing.T, method, target, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	if body == "" {
		c.Request = httptest.NewRequest(method, target, nil)
	} else {
		c.Request = httptest.NewRequest(method, target, strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
	}
	return c, w
}

// decodeGinResponse 解析 Gin 响应体为 JSON 对象
func decodeGinResponse(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应体失败: %v，body: %s", err, w.Body.String())
	}
	return resp
}

// TestWriteSuccessWithoutData 验证无数据时只写 dtm_result
func TestWriteSuccessWithoutData(t *testing.T) {
	c, w := newGinContext(t, http.MethodPost, "/dtm/action", "")
	WriteSuccess(c, nil)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d", w.Code)
	}
	resp := decodeGinResponse(t, w)
	if resp["dtm_result"] != dtmcli.ResultSuccess {
		t.Fatalf("dtm_result 期望 %q，实际 %v", dtmcli.ResultSuccess, resp["dtm_result"])
	}
	if _, ok := resp["data"]; ok {
		t.Fatalf("data 为 nil 时不应输出 data 字段: %s", w.Body.String())
	}
}

// TestWriteSuccessWithData 验证有数据时同时输出 data
func TestWriteSuccessWithData(t *testing.T) {
	c, w := newGinContext(t, http.MethodPost, "/dtm/action", "")
	WriteSuccess(c, map[string]string{"name": "x"})

	resp := decodeGinResponse(t, w)
	if resp["dtm_result"] != dtmcli.ResultSuccess {
		t.Fatalf("dtm_result 期望 %q，实际 %v", dtmcli.ResultSuccess, resp["dtm_result"])
	}
	data, ok := resp["data"].(map[string]any)
	if !ok || data["name"] != "x" {
		t.Fatalf("data 期望包含 name=x，实际 %v", resp["data"])
	}
}

// TestWriteFailureWithoutError 验证 err 为 nil 时只写 dtm_result
func TestWriteFailureWithoutError(t *testing.T) {
	c, w := newGinContext(t, http.MethodPost, "/dtm/action", "")
	WriteFailure(c, nil)

	if w.Code != http.StatusConflict {
		t.Fatalf("期望 409，实际 %d", w.Code)
	}
	resp := decodeGinResponse(t, w)
	if resp["dtm_result"] != dtmcli.ResultFailure {
		t.Fatalf("dtm_result 期望 %q，实际 %v", dtmcli.ResultFailure, resp["dtm_result"])
	}
	if _, ok := resp["message"]; ok {
		t.Fatalf("err 为 nil 时不应输出 message: %s", w.Body.String())
	}
	if _, ok := resp["code"]; ok {
		t.Fatalf("err 为 nil 时不应输出 code: %s", w.Body.String())
	}
}

// TestWriteFailurePlainError 验证普通错误输出 message 且不带 code
func TestWriteFailurePlainError(t *testing.T) {
	c, w := newGinContext(t, http.MethodPost, "/dtm/action", "")
	WriteFailure(c, errors.New("boom"))

	resp := decodeGinResponse(t, w)
	if resp["dtm_result"] != dtmcli.ResultFailure {
		t.Fatalf("dtm_result 期望 %q，实际 %v", dtmcli.ResultFailure, resp["dtm_result"])
	}
	if msg, _ := resp["message"].(string); !strings.Contains(msg, "boom") {
		t.Fatalf("message 应包含原始错误，实际 %v", resp["message"])
	}
	if _, ok := resp["code"]; ok {
		t.Fatalf("普通错误不应输出 code: %s", w.Body.String())
	}
}

// TestWriteFailureBizError 验证 BizError 的 Code 被提取进响应
func TestWriteFailureBizError(t *testing.T) {
	c, w := newGinContext(t, http.MethodPost, "/dtm/action", "")
	WriteFailure(c, errors.NewBiz("BIZ_001", "biz failed"))

	resp := decodeGinResponse(t, w)
	if got, _ := resp["code"].(string); got != "BIZ_001" {
		t.Fatalf("code 期望 BIZ_001，实际 %v", resp["code"])
	}
	if msg, _ := resp["message"].(string); !strings.Contains(msg, "biz failed") {
		t.Fatalf("message 应包含错误描述，实际 %v", resp["message"])
	}
}

// TestBindAndHandleActionSuccess 验证绑定请求体后附加 Barrier 并执行业务
func TestBindAndHandleActionSuccess(t *testing.T) {
	target := "/dtm/action?" + barrierQuery("g-handler", "b1", "action").Encode()
	c, w := newGinContext(t, http.MethodPost, target, `{"name":"x"}`)

	executed := 0
	var gotGid string
	var gotBarrier bool
	BindAndHandleAction(c, func(ctx context.Context, input *map[string]any) (map[string]any, error) {
		executed++
		bb, ok := BarrierFromContext(ctx)
		gotBarrier = ok
		if ok {
			gotGid = bb.Gid
		}
		return map[string]any{"echo": (*input)["name"]}, nil
	})

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d，body: %s", w.Code, w.Body.String())
	}
	if executed != 1 {
		t.Fatalf("业务应执行 1 次，实际 %d", executed)
	}
	if !gotBarrier || gotGid != "g-handler" {
		t.Fatalf("execute 收到的 ctx 应携带 Barrier，gotBarrier=%v gid=%q", gotBarrier, gotGid)
	}
	resp := decodeGinResponse(t, w)
	if resp["dtm_result"] != dtmcli.ResultSuccess {
		t.Fatalf("dtm_result 期望 %q，实际 %v", dtmcli.ResultSuccess, resp["dtm_result"])
	}
	data, _ := resp["data"].(map[string]any)
	if data["echo"] != "x" {
		t.Fatalf("data.echo 期望 x，实际 %v", resp["data"])
	}
}

// TestBindAndHandleActionBindError 验证 JSON 绑定失败时不执行业务并返回失败响应
func TestBindAndHandleActionBindError(t *testing.T) {
	c, w := newGinContext(t, http.MethodPost, "/dtm/action", `not-json`)

	executed := 0
	BindAndHandleAction(c, func(ctx context.Context, input *map[string]any) (map[string]any, error) {
		executed++
		return nil, nil
	})

	if w.Code != http.StatusConflict {
		t.Fatalf("期望 409，实际 %d", w.Code)
	}
	if executed != 0 {
		t.Fatalf("绑定失败不应执行业务，实际 %d", executed)
	}
	resp := decodeGinResponse(t, w)
	if resp["dtm_result"] != dtmcli.ResultFailure {
		t.Fatalf("dtm_result 期望 %q，实际 %v", dtmcli.ResultFailure, resp["dtm_result"])
	}
	if _, ok := resp["message"]; !ok {
		t.Fatalf("绑定失败应输出 message: %s", w.Body.String())
	}
}

// TestBindAndHandleActionBarrierError 验证 Barrier 元数据缺失时不执行业务并返回失败响应
func TestBindAndHandleActionBarrierError(t *testing.T) {
	c, w := newGinContext(t, http.MethodPost, "/dtm/action?op=action", `{"name":"x"}`)

	executed := 0
	BindAndHandleAction(c, func(ctx context.Context, input *map[string]any) (map[string]any, error) {
		executed++
		return nil, nil
	})

	if w.Code != http.StatusConflict {
		t.Fatalf("期望 409，实际 %d", w.Code)
	}
	if executed != 0 {
		t.Fatalf("Barrier 解析失败不应执行业务，实际 %d", executed)
	}
	if resp := decodeGinResponse(t, w); resp["dtm_result"] != dtmcli.ResultFailure {
		t.Fatalf("dtm_result 期望 %q，实际 %v", dtmcli.ResultFailure, resp["dtm_result"])
	}
}

// TestBindAndHandleActionExecuteError 验证业务错误（含 BizError）转为失败响应
func TestBindAndHandleActionExecuteError(t *testing.T) {
	cases := []struct {
		name     string
		execute  func(context.Context, *map[string]any) (map[string]any, error)
		wantCode any
	}{
		{
			name: "普通错误不带 code",
			execute: func(ctx context.Context, input *map[string]any) (map[string]any, error) {
				return nil, errors.New("boom")
			},
			wantCode: nil,
		},
		{
			name: "BizError 提取 code",
			execute: func(ctx context.Context, input *map[string]any) (map[string]any, error) {
				return nil, errors.NewBiz("BIZ_002", "biz failed")
			},
			wantCode: "BIZ_002",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := "/dtm/action?" + barrierQuery("g-handler", "b1", "action").Encode()
			c, w := newGinContext(t, http.MethodPost, target, `{"name":"x"}`)
			BindAndHandleAction(c, tc.execute)

			if w.Code != http.StatusConflict {
				t.Fatalf("期望 409，实际 %d", w.Code)
			}
			resp := decodeGinResponse(t, w)
			if resp["dtm_result"] != dtmcli.ResultFailure {
				t.Fatalf("dtm_result 期望 %q，实际 %v", dtmcli.ResultFailure, resp["dtm_result"])
			}
			if resp["code"] != tc.wantCode {
				t.Fatalf("code 期望 %v，实际 %v", tc.wantCode, resp["code"])
			}
		})
	}
}

// TestBindAndHandleCompensateBindError 验证补偿绑定失败时不执行业务
func TestBindAndHandleCompensateBindError(t *testing.T) {
	c, w := newGinContext(t, http.MethodPost, "/dtm/compensate", `not-json`)

	executed := 0
	BindAndHandleCompensate(c, func(ctx context.Context, input *map[string]any) error {
		executed++
		return nil
	})

	if w.Code != http.StatusConflict {
		t.Fatalf("期望 409，实际 %d", w.Code)
	}
	if executed != 0 {
		t.Fatalf("绑定失败不应执行补偿，实际 %d", executed)
	}
	if resp := decodeGinResponse(t, w); resp["dtm_result"] != dtmcli.ResultFailure {
		t.Fatalf("dtm_result 期望 %q，实际 %v", dtmcli.ResultFailure, resp["dtm_result"])
	}
}

// TestBindAndHandleCompensateBarrierError 验证补偿 Barrier 解析失败时不执行业务
func TestBindAndHandleCompensateBarrierError(t *testing.T) {
	c, w := newGinContext(t, http.MethodPost, "/dtm/compensate?op=compensate", `{"name":"x"}`)

	executed := 0
	BindAndHandleCompensate(c, func(ctx context.Context, input *map[string]any) error {
		executed++
		return nil
	})

	if w.Code != http.StatusConflict {
		t.Fatalf("期望 409，实际 %d", w.Code)
	}
	if executed != 0 {
		t.Fatalf("Barrier 解析失败不应执行补偿，实际 %d", executed)
	}
	if resp := decodeGinResponse(t, w); resp["dtm_result"] != dtmcli.ResultFailure {
		t.Fatalf("dtm_result 期望 %q，实际 %v", dtmcli.ResultFailure, resp["dtm_result"])
	}
}

// TestBindAndHandleCompensateSuccess 验证补偿成功时输出无 data 的成功响应
func TestBindAndHandleCompensateSuccess(t *testing.T) {
	target := "/dtm/compensate?" + barrierQuery("g-handler", "b1", "compensate").Encode()
	c, w := newGinContext(t, http.MethodPost, target, `{"name":"x"}`)

	executed := 0
	BindAndHandleCompensate(c, func(ctx context.Context, input *map[string]any) error {
		executed++
		return nil
	})

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d，body: %s", w.Code, w.Body.String())
	}
	if executed != 1 {
		t.Fatalf("补偿应执行 1 次，实际 %d", executed)
	}
	resp := decodeGinResponse(t, w)
	if resp["dtm_result"] != dtmcli.ResultSuccess {
		t.Fatalf("dtm_result 期望 %q，实际 %v", dtmcli.ResultSuccess, resp["dtm_result"])
	}
	if _, ok := resp["data"]; ok {
		t.Fatalf("补偿成功不应输出 data: %s", w.Body.String())
	}
}

// TestBindAndHandleCompensateExecuteError 验证补偿失败时返回失败响应
func TestBindAndHandleCompensateExecuteError(t *testing.T) {
	target := "/dtm/compensate?" + barrierQuery("g-handler", "b1", "compensate").Encode()
	c, w := newGinContext(t, http.MethodPost, target, `{"name":"x"}`)

	executed := 0
	BindAndHandleCompensate(c, func(ctx context.Context, input *map[string]any) error {
		executed++
		return errors.New("compensate boom")
	})

	if w.Code != http.StatusConflict {
		t.Fatalf("期望 409，实际 %d", w.Code)
	}
	if executed != 1 {
		t.Fatalf("补偿应执行 1 次，实际 %d", executed)
	}
	resp := decodeGinResponse(t, w)
	if resp["dtm_result"] != dtmcli.ResultFailure {
		t.Fatalf("dtm_result 期望 %q，实际 %v", dtmcli.ResultFailure, resp["dtm_result"])
	}
	if msg, _ := resp["message"].(string); !strings.Contains(msg, "compensate boom") {
		t.Fatalf("message 应包含原始错误，实际 %v", resp["message"])
	}
}
