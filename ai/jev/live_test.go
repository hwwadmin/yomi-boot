// 按填写的实网配置调用门面
//
// @author zwei

package jev

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// liveJevConfig 千问决策模型（System One）；实网测前填 APIURL、APIKey、Model
var liveJevConfig = Config{
	Protocol: ProtocolSystemOne,
	APIURL:   "https://maas.qianwenaiapi.com/compatible-mode/v1/systemone",
	APIKey:   "",
	Model:    "decision-model-preview",
	Timeout:  60 * time.Second,
}

// requireLiveJevConfig 校验实网配置已填密钥与地址；未填则跳过当前子测试
func requireLiveJevConfig(t *testing.T, cfg Config) Config {
	t.Helper()
	if strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.APIURL) == "" {
		t.Skip("填写 liveJevConfig 的 APIURL 与 APIKey 后再跑")
	}
	return cfg
}

// TestDecide 千问 decision-model-preview 实网决策（与官方 curl 示例同形）
func TestDecide(t *testing.T) {
	cli, err := New(requireLiveJevConfig(t, liveJevConfig))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	resp, err := cli.Decide(context.Background(), DecideRequest{
		State: json.RawMessage(`{"ticket_id":"T-1001","content":"订单支付后超过 24 小时仍未到账，用户无法继续使用核心服务，要求立即处理。"}`),
		Questions: map[string]Question{
			"department": {
				Type:         QuestionChoice,
				Instructions: "应该由哪个团队处理？",
				Criteria:     json.RawMessage(`{"billing":"支付、退款和账单问题","technical":"产品故障和集成问题"}`),
			},
			"escalate": {
				Type:         QuestionNoul,
				Instructions: "是否需要立即通知值班人员？",
			},
			"severity": {
				Type:         QuestionScore,
				Instructions: "这个问题有多严重？",
				Criteria:     json.RawMessage(`["轻微问题，不影响功能","部分功能受影响，但存在替代方案","核心功能不可用，没有替代方案","造成严重业务或安全影响"]`),
			},
		},
	})
	if err != nil {
		t.Logf("Decide error (full): %+v", err)
		t.Fatalf("Decide: %v", err)
	}
	logDecideResponse(t, "Decide response", resp)
	if len(resp.Answers) != 3 {
		t.Fatalf("expected 3 answers, got %d", len(resp.Answers))
	}
	dept, ok := resp.Answers["department"]
	if !ok || dept.Type != "choice" || dept.Choice == nil {
		t.Fatalf("missing or invalid department answer")
	}
	esc, ok := resp.Answers["escalate"]
	if !ok || esc.Type != "noul" || esc.Noul == nil {
		t.Fatalf("missing or invalid escalate answer")
	}
}
