package dtmtest_test

import (
	"context"
	"encoding/json"
	"github.com/dtm-labs/client/dtmcli"
	"github.com/hwwadmin/yomi-boot/component/cloud/dtm/dtmtest"
	"net"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

// TestSagaSubmitSucceeds 验证真实 DTM 能生成 gid、调用分支并完成 Saga。
func TestSagaSubmitSucceeds(t *testing.T) {
	client, cleanup := dtmtest.NewClient(t)
	defer cleanup()

	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("监听回调端口失败: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	var compensated atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/action", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(dtmcli.ResultSuccess))
	})
	mux.HandleFunc("/compensate", func(w http.ResponseWriter, _ *http.Request) {
		compensated.Store(true)
		_, _ = w.Write([]byte(dtmcli.ResultSuccess))
	})
	httpServer := &http.Server{Handler: mux}
	go func() { _ = httpServer.Serve(ln) }()
	t.Cleanup(func() { _ = httpServer.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	saga, err := client.NewSaga(ctx)
	if err != nil {
		t.Fatalf("创建 Saga 失败: %v", err)
	}
	if saga.Gid == "" {
		t.Fatal("DTM 返回的 gid 为空")
	}

	base := dtmtest.CallbackBase(port)
	if err := saga.Add(base+"/action", base+"/compensate", map[string]string{"order": "1"}).Submit(); err != nil {
		t.Fatalf("提交 Saga 失败: %v", err)
	}

	deadline := time.Now().Add(15 * time.Second)
	var status string
	for time.Now().Before(deadline) {
		status, err = queryStatus(ctx, saga.Dtm, saga.Gid)
		if err != nil {
			t.Fatalf("查询事务失败: %v", err)
		}
		if status == dtmcli.StatusSucceed {
			break
		}
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			t.Fatalf("等待事务完成超时: %v", ctx.Err())
		case <-timer.C:
		}
	}
	if status != dtmcli.StatusSucceed {
		t.Fatalf("事务状态 = %q, want %q", status, dtmcli.StatusSucceed)
	}
	if compensated.Load() {
		t.Fatal("成功事务不应调用补偿分支")
	}
}

func queryStatus(ctx context.Context, server, gid string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server+"/query?gid="+url.QueryEscape(gid), nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		Transaction struct {
			Status string `json:"status"`
		} `json:"transaction"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return body.Transaction.Status, nil
}
