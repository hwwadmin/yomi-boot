// 文档版本治理：写入版本生成与检索侧状态加载。
//
// @author zwei

package rag

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/ai/vectordb"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/logger"
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

// loadDocumentStates 读取命中文档当前版本与块总数；辅助查询失败时降级为不过滤
func (e *engine) loadDocumentStates(ctx context.Context, collection string, hits []vectordb.Hit) (map[string]documentState, error) {
	ids := make([]string, 0, len(hits))
	seen := make(map[string]struct{}, len(hits))
	for _, hit := range hits {
		docID := strings.TrimSpace(hit.Metadata[MetadataKeyDocumentID])
		// 非本包写入的超长文档 ID 不参与块数查询，避免 Get 因主键超长整体失败
		if docID == "" || len(docID) > maxDocIDLen {
			continue
		}
		if _, ok := seen[docID]; ok {
			continue
		}
		seen[docID] = struct{}{}
		ids = append(ids, chunkID(docID, 0))
	}
	if len(ids) == 0 {
		return nil, nil
	}
	records, err := e.store.Get(ctx, collection, ids)
	if err != nil {
		// 取消/超时必须上抛，不能把 ctx 失效降级成正常结果
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		if errors.Is(err, vectordb.ErrNotFound) {
			return nil, err
		}
		// 其他辅助查询失败时降级为不过滤，避免检索整体失败
		logger.Warn(ctx, "ai/rag 读取文档版本失败，降级为不过滤旧块", "collection", collection, "error", err)
		return nil, nil
	}
	states := make(map[string]documentState, len(records))
	for _, record := range records {
		docID := strings.TrimSpace(record.Metadata[MetadataKeyDocumentID])
		if docID == "" {
			continue
		}
		total, err := strconv.Atoi(strings.TrimSpace(record.Metadata[MetadataKeyChunkCount]))
		if err != nil || total <= 0 {
			continue
		}
		states[docID] = documentState{
			total:   total,
			version: strings.TrimSpace(record.Metadata[MetadataKeyVersion]),
		}
	}
	return states, nil
}
