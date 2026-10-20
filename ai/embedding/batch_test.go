// 大批量输入分批单测。
//
// @author zwei

package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"
)

// embeddingBatchShim 记录每批输入条数并返回对应下标向量
type embeddingBatchShim struct {
	mu      sync.Mutex
	batches []int
}

// RoundTrip 解析请求体中的 input 并返回同长度向量
func (s *embeddingBatchShim) RoundTrip(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Input []string `json:"input"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.batches = append(s.batches, len(payload.Input))
	s.mu.Unlock()
	data := make([]map[string]any, len(payload.Input))
	for i := range payload.Input {
		data[i] = map[string]any{"object": "embedding", "index": i, "embedding": []float64{float64(i)}}
	}
	raw, err := json.Marshal(map[string]any{
		"object": "list",
		"data":   data,
		"model":  "m",
		"usage":  map[string]any{"prompt_tokens": len(payload.Input), "total_tokens": len(payload.Input)},
	})
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     http.StatusText(http.StatusOK),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(raw)),
		Request:    req,
	}, nil
}

// TestEmbedBatchesLargeInput 验证超过 2048 条时内部拆分并保持结果顺序与用量合计
func TestEmbedBatchesLargeInput(t *testing.T) {
	t.Parallel()
	shim := &embeddingBatchShim{}
	cli, err := New(Config{
		Protocol:   ProtocolOpenAI,
		APIURL:     "https://example.invalid/v1",
		APIKey:     "sk-test",
		Model:      "m",
		HTTPClient: &http.Client{Transport: shim},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	inputs := make([]string, maxEmbedBatch+2)
	for i := range inputs {
		inputs[i] = "x"
	}
	resp, err := cli.Embed(context.Background(), Request{Inputs: inputs})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(resp.Vectors) != len(inputs) {
		t.Fatalf("vectors=%d", len(resp.Vectors))
	}
	shim.mu.Lock()
	defer shim.mu.Unlock()
	if len(shim.batches) != 2 || shim.batches[0] != maxEmbedBatch || shim.batches[1] != 2 {
		t.Fatalf("batches=%v", shim.batches)
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != len(inputs) {
		t.Fatalf("usage=%+v", resp.Usage)
	}
}
