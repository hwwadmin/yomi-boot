// Package dtm 提供 DTM 启动探活能力
//
// @author zwei
package dtm

import (
	"io"
	"net/http"
	"strings"
	"time"
	"yomi-boot/appctx"
	"yomi-boot/logger"
)

const probeTimeout = 3 * time.Second

// ProbeServer 启动时对 DTM 服务器做一次可达性探测，结果写入日志，不影响启动流程。
func ProbeServer(server string) {
	server = strings.TrimSpace(server)
	if server == "" {
		return
	}

	versionURL := strings.TrimRight(server, "/") + "/version"
	ctx, cancel := appctx.BackgroundWithTimeout(probeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, versionURL, nil)
	if err != nil {
		logger.WarnWithoutCtx("dtm探活失败", "server", server, "url", versionURL, "err", err.Error())
		return
	}

	resp, err := (&http.Client{Timeout: probeTimeout}).Do(req)
	if err != nil {
		logger.WarnWithoutCtx("dtm探活失败", "server", server, "url", versionURL, "err", err.Error())
		return
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	bodyText := strings.TrimSpace(string(body))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		logger.WarnWithoutCtx(
			"dtm探活失败",
			"server", server,
			"url", versionURL,
			"status", resp.StatusCode,
			"body", bodyText,
		)
		return
	}

	logger.InfoWithoutCtx(
		"dtm探活成功",
		"server", server,
		"url", versionURL,
		"status", resp.StatusCode,
		"body", bodyText,
	)
}
