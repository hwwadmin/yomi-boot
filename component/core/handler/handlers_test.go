package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hwwadmin/yomi-boot/component/core/appctx"
	"github.com/hwwadmin/yomi-boot/component/core/errors"
	"github.com/hwwadmin/yomi-boot/component/core/handler"
	"github.com/hwwadmin/yomi-boot/component/core/response"
)

type nameInput struct {
	Name string `json:"name"`
}

func TestHandlersAndHealth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response.RegisterDefaultForTest(t)

	engine := gin.New()
	engine.GET("/ok", func(c *gin.Context) {
		handler.Handle(c, func(context.Context) (string, error) {
			return "pong", nil
		})
	})
	engine.GET("/fail", func(c *gin.Context) {
		handler.HandleVoid(c, func(context.Context) error {
			return errors.New("nope")
		})
	})
	engine.GET("/fail-data", func(c *gin.Context) {
		handler.Handle(c, func(context.Context) (string, error) {
			return "", errors.New("nope")
		})
	})
	engine.POST("/bind", func(c *gin.Context) {
		handler.BindAndHandle(c, func(_ context.Context, in *nameInput) (string, error) {
			return in.Name, nil
		})
	})
	engine.POST("/bind-void", func(c *gin.Context) {
		handler.BindAndHandleVoid(c, func(_ context.Context, in *nameInput) error {
			if in.Name == "" {
				return errors.New("empty")
			}
			return nil
		})
	})
	engine.POST("/bind-req", func(c *gin.Context) {
		handler.BindAndHandleWithRequest(c, func(_ context.Context, req *http.Request, in *nameInput) (string, error) {
			return req.Method + ":" + in.Name, nil
		})
	})
	engine.POST("/bad-void", func(c *gin.Context) {
		handler.BindAndHandleVoid(c, func(context.Context, *nameInput) error {
			return nil
		})
	})
	engine.POST("/bad-req", func(c *gin.Context) {
		handler.BindAndHandleWithRequest(c, func(context.Context, *http.Request, *nameInput) (string, error) {
			return "", nil
		})
	})
	engine.POST("/bad", func(c *gin.Context) {
		handler.BindAndHandle(c, func(context.Context, *nameInput) (string, error) {
			return "", nil
		})
	})
	engine.GET("/write", func(c *gin.Context) {
		handler.WriteSuccess(c, "ok")
	})
	engine.GET("/write-fail", func(c *gin.Context) {
		handler.WriteFailure(c, errors.New("fail"))
	})

	app := appctx.NewAppCtx("yomi", "test", "1")
	handler.NewHealthHandler(" /live ", "", nil).Routes(engine.Group(""))
	handler.NewHealthHandler("", " /ready ", app).Routes(engine.Group(""))
	handler.NewHealthHandler(" ", " ", nil).Routes(engine.Group(""))

	if err := handler.JSONBindError(&http.MaxBytesError{Limit: 8}); err == nil {
		t.Fatal("超限绑定错误不应为空")
	}

	get := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}
	post := func(path, body string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(rec, req)
		return rec
	}

	if rec := get("/ok"); !strings.Contains(rec.Body.String(), "pong") {
		t.Fatalf("Handle 响应 %s", rec.Body.String())
	}
	if rec := get("/fail"); !strings.Contains(rec.Body.String(), "nope") && rec.Body.Len() == 0 {
		t.Fatal("HandleVoid 失败应写出响应")
	}
	if rec := get("/fail-data"); rec.Body.Len() == 0 {
		t.Fatal("Handle 失败应写出响应")
	}
	if rec := post("/bind", `{"name":"ada"}`); !strings.Contains(rec.Body.String(), "ada") {
		t.Fatalf("BindAndHandle 响应 %s", rec.Body.String())
	}
	if rec := post("/bind-void", `{"name":"ada"}`); rec.Code == 0 {
		t.Fatal("BindAndHandleVoid 应写出响应")
	}
	if rec := post("/bind-void", `{"name":""}`); rec.Body.Len() == 0 {
		t.Fatal("BindAndHandleVoid 失败应写出响应")
	}
	if rec := post("/bind-req", `{"name":"ada"}`); !strings.Contains(rec.Body.String(), "POST:ada") {
		t.Fatalf("BindAndHandleWithRequest 响应 %s", rec.Body.String())
	}
	if rec := post("/bad", `{`); rec.Body.Len() == 0 {
		t.Fatal("非法 JSON 应写出绑定错误")
	}
	if rec := post("/bad-void", `{`); rec.Body.Len() == 0 {
		t.Fatal("BindAndHandleVoid 非法 JSON 应写出绑定错误")
	}
	if rec := post("/bad-req", `{`); rec.Body.Len() == 0 {
		t.Fatal("BindAndHandleWithRequest 非法 JSON 应写出绑定错误")
	}
	if rec := get("/write"); !strings.Contains(rec.Body.String(), "ok") {
		t.Fatalf("WriteSuccess 响应 %s", rec.Body.String())
	}
	if rec := get("/write-fail"); rec.Body.Len() == 0 {
		t.Fatal("WriteFailure 应写出响应")
	}
	if rec := get("/live"); !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("consul 探针 %s", rec.Body.String())
	}
	if rec := get("/ready"); !strings.Contains(rec.Body.String(), "yomi") {
		t.Fatalf("运维探针 %s", rec.Body.String())
	}
}
