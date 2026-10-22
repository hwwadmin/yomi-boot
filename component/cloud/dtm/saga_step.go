// Package dtm 提供 DTM Saga 步骤构建辅助
package dtm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/shared"
	"gitee.com/hq_project/yomi-boot/component/core/traceid"

	"github.com/dtm-labs/client/dtmcli"
)

// maxErrorBodyRunes 是错误信息中保留的响应体最大字符数
const maxErrorBodyRunes = 256

// NewSaga 创建已注入 traceID 的 DTM Saga 实例，控制面请求使用 Client 的超时
func (c *Client) NewSaga(ctx context.Context) (*dtmcli.Saga, error) {
	if !c.Enabled() {
		return nil, errors.Sentinel(ErrNotConfigured)
	}
	gid, err := c.genGid(ctx)
	if err != nil {
		return nil, err
	}
	saga := dtmcli.NewSaga(c.server, gid)
	// dtmcli 的控制面超时单位是秒，不足 1 秒按 1 秒处理，避免 0 被当成不超时
	timeoutSeconds := int64(c.effectiveTimeout() / time.Second)
	if timeoutSeconds < 1 {
		timeoutSeconds = 1
	}
	saga.WithGlobalTransRequestTimeout(timeoutSeconds)
	saga.BranchHeaders = map[string]string{shared.TraceIDHeader: traceid.FromContext(ctx)}
	return saga, nil
}

// AddSagaStep 通过 Consul 解析目标服务端点地址，向 Saga 追加 Action/Compensate 步骤
// serviceName 为目标微服务的 Consul 注册名；saga 不能为 nil；解析出的地址必须是 http/https 绝对地址
func (c *Client) AddSagaStep(ctx context.Context, saga *dtmcli.Saga,
	serviceName, actionEndpoint, compensateEndpoint string, payload any,
) error {
	if !c.Enabled() {
		return errors.Sentinel(ErrNotConfigured)
	}
	if saga == nil {
		return errors.New("dtm: saga 不能为 nil")
	}
	if c.consul == nil {
		return errors.New("dtm: consul 客户端未初始化")
	}
	actionURL, err := c.consul.ResolveHTTPBase(ctx, serviceName, actionEndpoint)
	if err != nil {
		return err
	}
	if err := validateHTTPURL("action", actionURL); err != nil {
		return err
	}
	compensateURL, err := c.consul.ResolveHTTPBase(ctx, serviceName, compensateEndpoint)
	if err != nil {
		return err
	}
	if err := validateHTTPURL("compensate", compensateURL); err != nil {
		return err
	}
	return appendSagaStep(saga, actionURL, compensateURL, payload)
}

// appendSagaStep 先序列化 payload 再追加分支，避免 dtmcli 的 MustMarshalString 在失败时 panic
func appendSagaStep(saga *dtmcli.Saga, actionURL, compensateURL string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "dtm: 序列化 Saga 分支 payload 失败")
	}
	saga.Add(actionURL, compensateURL, json.RawMessage(raw))
	return nil
}

// genGid 从 DTM 服务端申请全局事务 ID
func (c *Client) genGid(ctx context.Context) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	url := strings.TrimRight(c.server, "/") + "/newGid"
	resp, err := dtmcli.GetRestyClient2(c.effectiveTimeout()).R().SetContext(ctx).Get(url)
	if err != nil {
		return "", errors.Sentinel(fmt.Errorf("%w: %w", ErrGenGidFailed, err))
	}
	if resp.StatusCode() != http.StatusOK {
		return "", errors.Sentinel(fmt.Errorf("%w: status %d, resp: %s",
			ErrGenGidFailed, resp.StatusCode(), truncateBody(resp.String())))
	}
	var res struct {
		Gid string `json:"gid"`
	}
	if err := json.Unmarshal(resp.Body(), &res); err != nil {
		return "", errors.Sentinel(fmt.Errorf("%w: %w", ErrGenGidFailed, err))
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
