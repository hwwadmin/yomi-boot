// Package dtm 提供 DTM Saga 步骤构建辅助
package dtm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/boot/errors"
	"gitee.com/hq_project/yomi-boot/boot/shared"
	"gitee.com/hq_project/yomi-boot/boot/traceid"
	"gitee.com/hq_project/yomi-boot/cloud/consul"

	"github.com/dtm-labs/client/dtmcli"
)

// DefaultRequestTimeout 是 DTM 控制面请求（newGid/submit/abort/query）的默认超时
// 该值同时作为 DTM 服务端回调各分支的请求超时，分支可能超过 3 秒时用 NewSagaWithTimeout 调大
const DefaultRequestTimeout = 3 * time.Second

// maxErrorBodyRunes 是错误信息中保留的响应体最大字符数
const maxErrorBodyRunes = 256

// AddSagaStep 通过 Consul 解析目标服务端点地址，向 Saga 追加 Action/Compensate 步骤
// consulClient 用于服务发现，serviceName 为目标微服务的 Consul 注册名
func AddSagaStep(ctx context.Context, saga *dtmcli.Saga, consulClient *consul.Client,
	serviceName, actionEndpoint, compensateEndpoint string, payload any,
) error {
	actionURL, err := consulClient.ResolveHTTPBase(ctx, serviceName, actionEndpoint)
	if err != nil {
		return err
	}
	compensateURL, err := consulClient.ResolveHTTPBase(ctx, serviceName, compensateEndpoint)
	if err != nil {
		return err
	}
	saga.Add(actionURL, compensateURL, payload)
	return nil
}

// NewSaga 创建已注入 traceID 的 DTM Saga 实例，控制面请求使用 DefaultRequestTimeout
func NewSaga(ctx context.Context, dtmServer string) (*dtmcli.Saga, error) {
	return NewSagaWithTimeout(ctx, dtmServer, DefaultRequestTimeout)
}

// NewSagaWithTimeout 创建已注入 traceID 的 DTM Saga 实例
// timeout 同时约束 newGid 与 submit/abort/query 等控制面请求，并作为 DTM 服务端回调各分支的请求超时
// timeout <= 0 时使用 DefaultRequestTimeout
func NewSagaWithTimeout(ctx context.Context, dtmServer string, timeout time.Duration) (*dtmcli.Saga, error) {
	if timeout <= 0 {
		timeout = DefaultRequestTimeout
	}
	gid, err := genGid(ctx, dtmServer, timeout)
	if err != nil {
		return nil, err
	}
	saga := dtmcli.NewSaga(dtmServer, gid)
	// dtmcli 的控制面超时单位是秒，不足 1 秒按 1 秒处理，避免 0 被当成不超时
	timeoutSeconds := int64(timeout / time.Second)
	if timeoutSeconds < 1 {
		timeoutSeconds = 1
	}
	saga.WithGlobalTransRequestTimeout(timeoutSeconds)
	saga.BranchHeaders = map[string]string{shared.TraceIDHeader: traceid.FromContext(ctx)}
	return saga, nil
}

// genGid 从 DTM 服务端申请全局事务 ID
func genGid(ctx context.Context, server string, timeout time.Duration) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	url := strings.TrimRight(server, "/") + "/newGid"
	resp, err := dtmcli.GetRestyClient2(timeout).R().SetContext(ctx).Get(url)
	if err != nil {
		return "", errors.Sentinel(fmt.Errorf("%w: %v", ErrGenGidFailed, err))
	}
	if resp.StatusCode() != http.StatusOK {
		return "", errors.Sentinel(fmt.Errorf("%w: status %d, resp: %s",
			ErrGenGidFailed, resp.StatusCode(), truncateBody(resp.String())))
	}
	var res struct {
		Gid string `json:"gid"`
	}
	if err := json.Unmarshal(resp.Body(), &res); err != nil {
		return "", errors.Sentinel(fmt.Errorf("%w: %v", ErrGenGidFailed, err))
	}
	if res.Gid == "" {
		return "", errors.Sentinel(fmt.Errorf("%w: empty gid, resp: %s", ErrGenGidFailed, truncateBody(resp.String())))
	}
	return res.Gid, nil
}

// truncateBody 截断响应体，避免错误信息携带过长的报文
func truncateBody(body string) string {
	body = strings.TrimSpace(body)
	runes := []rune(body)
	if len(runes) <= maxErrorBodyRunes {
		return body
	}
	return string(runes[:maxErrorBodyRunes]) + "..."
}
