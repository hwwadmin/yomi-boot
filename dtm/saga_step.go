// Package dtm 提供 DTM Saga 步骤构建辅助
package dtm

import (
	"context"
	"encoding/json"
	"fmt"
	"gitee.com/hq_project/yomi-boot/consul"
	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/shared"
	"gitee.com/hq_project/yomi-boot/traceid"
	"strings"

	"github.com/dtm-labs/client/dtmcli"
)

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

// NewSaga 创建已注入 traceID 的 DTM Saga 实例，BranchHeaders 自动填充当前 ctx 中的链路 ID
func NewSaga(ctx context.Context, dtmServer string) (*dtmcli.Saga, error) {
	gid, err := genGid(dtmServer)
	if err != nil {
		return nil, err
	}
	saga := dtmcli.NewSaga(dtmServer, gid)
	saga.BranchHeaders = map[string]string{shared.TraceIDHeader: traceid.FromContext(ctx)}
	return saga, nil
}

// genGid 从 DTM 服务端申请全局事务 ID
func genGid(server string) (string, error) {
	url := strings.TrimRight(server, "/") + "/newGid"
	resp, err := dtmcli.GetRestyClient().R().Get(url)
	if err != nil {
		return "", errors.Sentinel(fmt.Errorf("%w: %v", ErrGenGidFailed, err))
	}
	var res struct {
		Gid string `json:"gid"`
	}
	if err := json.Unmarshal(resp.Body(), &res); err != nil {
		return "", errors.Sentinel(fmt.Errorf("%w: %v", ErrGenGidFailed, err))
	}
	if res.Gid == "" {
		return "", errors.Sentinel(fmt.Errorf("%w: empty gid, resp: %s", ErrGenGidFailed, resp))
	}
	return res.Gid, nil
}
