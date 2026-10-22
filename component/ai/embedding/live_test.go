// 按填写的实网配置调用门面
//
// @author zwei

package embedding

import (
	"context"
	"strings"
	"testing"
	"time"
)

// liveEmbeddingConfig 实网 OpenAI Embeddings 配置；测前填 APIURL、Model；APIKey 可空
var liveEmbeddingConfig = Config{
	Protocol: ProtocolOpenAI,
	APIURL:   "https://oai.endpoints.kepler.ai.cloud.ovh.net/v1",
	APIKey:   "",
	Model:    "bge-m3",
	Timeout:  60 * time.Second,
}

// requireLiveEmbeddingConfig 校验已填 APIURL；未填则跳过。APIKey 可空
func requireLiveEmbeddingConfig(t *testing.T, cfg Config) Config {
	t.Helper()
	if strings.TrimSpace(cfg.APIURL) == "" {
		t.Skip("填写 liveEmbeddingConfig 的 APIURL 后再跑；APIKey 可空")
	}
	return cfg
}

// TestEmbed 用 OpenAI Embeddings 配置发起同步向量化
func TestEmbed(t *testing.T) {
	cli, err := New(requireLiveEmbeddingConfig(t, liveEmbeddingConfig))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	resp, err := cli.Embed(context.Background(), Request{
		Inputs: []string{"hello", "world"},
	})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if resp == nil || len(resp.Vectors) != 2 {
		t.Fatalf("resp=%+v", resp)
	}
	for i, v := range resp.Vectors {
		if len(v) == 0 {
			t.Fatalf("empty vector at %d", i)
		}
	}
	t.Logf("model=%s dim=%d usage=%+v", resp.Model, len(resp.Vectors[0]), resp.Usage)
}
