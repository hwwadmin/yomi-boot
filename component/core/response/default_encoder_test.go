package response

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/core/errors"

	"github.com/gin-gonic/gin"
)

// TestDefaultEncoder_SuccessAndFailure 验证成功与失败响应编码
func TestDefaultEncoder_SuccessAndFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	RegisterDefaultForTest(t)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	Enc().EncodeSuccess(c, map[string]string{"id": "1"})
	body := rec.Body.String()
	if !strings.Contains(body, `"code":"OK"`) || !strings.Contains(body, `"msg":"success"`) {
		t.Fatalf("成功响应: %s", body)
	}

	rec2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rec2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	Enc().EncodeFailure(c2, errors.New("TOKEN_REQUIRED"))
	if !strings.Contains(rec2.Body.String(), `"code":"TOKEN_REQUIRED"`) {
		t.Fatalf("失败响应: %s", rec2.Body.String())
	}
}
