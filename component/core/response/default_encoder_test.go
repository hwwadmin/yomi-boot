package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hwwadmin/yomi-boot/component/core/errors"
	"github.com/hwwadmin/yomi-boot/component/core/shared/dto"

	"github.com/gin-gonic/gin"
)

// newTestContext 构造测试用 gin 上下文
func newTestContext(rec *httptest.ResponseRecorder) *gin.Context {
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	return c
}

// TestDefaultEncoder_SuccessAndFailure 验证成功与失败响应编码
func TestDefaultEncoder_SuccessAndFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	RegisterDefaultForTest(t)

	rec := httptest.NewRecorder()
	Enc().EncodeSuccess(newTestContext(rec), map[string]string{"id": "1"})
	body := rec.Body.String()
	if !strings.Contains(body, `"code":"OK"`) || !strings.Contains(body, `"msg":"success"`) {
		t.Fatalf("成功响应: %s", body)
	}

	rec2 := httptest.NewRecorder()
	Enc().EncodeFailure(newTestContext(rec2), errors.New("TOKEN_REQUIRED"))
	if !strings.Contains(rec2.Body.String(), `"code":"TOKEN_REQUIRED"`) {
		t.Fatalf("失败响应: %s", rec2.Body.String())
	}
}

// TestDefaultEncoder_PageUnderData 验证分页载荷整体落在 data 下，顶层不出现 total
func TestDefaultEncoder_PageUnderData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	RegisterDefaultForTest(t)

	rec := httptest.NewRecorder()
	Enc().EncodeSuccess(newTestContext(rec), dto.NewPageOutput([]string{"a"}, 1))

	var body map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if _, ok := body["total"]; ok {
		t.Fatalf("顶层不应出现 total: %s", rec.Body.String())
	}

	var page dto.PageOutput[string]
	if err := json.Unmarshal(body["data"], &page); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0] != "a" || page.Total != 1 {
		t.Fatalf("data 应为 {items,total}: %s", body["data"])
	}

	parsed, ok := Enc().Parse(rec.Body.Bytes())
	if !ok || !Enc().IsSuccess(parsed.Code) {
		t.Fatalf("Parse 应识别本实现写出的 body: %+v ok=%v", parsed, ok)
	}
}
