// 文档版本治理：写入版本生成与版本状态判定。
//
// @author zwei

package rag

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/ai/vectordb"
)

// documentState 是一次写入的版本与块总数
type documentState struct {
	total   int
	version string
}

// newVersion 生成文档写入版本；检索侧据此区分新旧块
func newVersion() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// isCurrentChunk 判断命中是否为文档当前版本的有效块；无状态信息的命中放行
func isCurrentChunk(hit vectordb.Hit, states map[string]documentState) bool {
	state, ok := states[strings.TrimSpace(hit.Metadata[MetadataKeyDocumentID])]
	if !ok {
		return true
	}
	if version := strings.TrimSpace(hit.Metadata[MetadataKeyVersion]); state.version != "" && version != state.version {
		return false
	}
	index, err := strconv.Atoi(strings.TrimSpace(hit.Metadata[MetadataKeyChunkIndex]))
	if err != nil {
		return true
	}
	return index < state.total
}
