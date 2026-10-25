// 雪花 ID 生成器配置的单元测试
//
// @author zwei
package snowflake

import "testing"

// TestConfigValidate 验证工作节点 ID 的合法域
func TestConfigValidate(t *testing.T) {
	cases := []struct {
		name     string
		workerID int
		wantErr  bool
	}{
		{name: "下界 0", workerID: 0},
		{name: "上界 1023", workerID: 1023},
		{name: "负数", workerID: -1, wantErr: true},
		{name: "超上限", workerID: 1024, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := (Config{WorkerID: tc.workerID}).Validate()
			if tc.wantErr && err == nil {
				t.Fatalf("workerID=%d 应返回错误", tc.workerID)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("workerID=%d 不应报错: %v", tc.workerID, err)
			}
		})
	}
}
