// Package handler 提供健康检查与运维探针 Handler
//
// @author zwei
package handler

import (
	"net/http"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/common/appctx"

	"github.com/gin-gonic/gin"
)

// HealthResponseData 表示健康检查响应数据
type HealthResponseData struct {
	// Name 表示应用名称
	Name string `json:"name"`
	// Env 表示当前运行环境
	Env string `json:"env"`
	// Version 表示应用版本
	Version string `json:"version"`
}

// HealthHandler 注册 Consul 探针与运维探针路由
type HealthHandler struct {
	consulPath string
	opsPath    string
	appCtx     appctx.AppCtx
}

// NewHealthHandler 创建 HealthHandler；consulPath 为 Consul 轻量探针，opsPath 为运维详情探针（需 appCtx）
func NewHealthHandler(consulPath, opsPath string, appCtx appctx.AppCtx) *HealthHandler {
	return &HealthHandler{
		consulPath: strings.TrimSpace(consulPath),
		opsPath:    strings.TrimSpace(opsPath),
		appCtx:     appCtx,
	}
}

// Routes 注册健康检查路由
func (h *HealthHandler) Routes(router *gin.RouterGroup) {
	// 注册 Consul 轻量探针路由
	if h.consulPath != "" {
		router.GET(h.consulPath, h.handleConsulHealth)
	}
	// 注册运维详情探针路由
	if h.opsPath != "" && h.appCtx != nil {
		router.GET(h.opsPath, h.handleOpsHealth)
	}
}

// handleConsulHealth Consul 探针：仅返回进程存活，供 Consul HTTP Check 使用
func (h *HealthHandler) handleConsulHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// handleOpsHealth 运维探针：返回应用名称、环境、版本等运维信息
func (h *HealthHandler) handleOpsHealth(c *gin.Context) {
	WriteSuccess(c, HealthResponseData{
		Name: h.appCtx.Name(), Env: h.appCtx.Env(), Version: h.appCtx.Version(),
	})
}
