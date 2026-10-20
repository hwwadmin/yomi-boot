// 公共入参校验与规范化。
//
// @author zwei

package rag

import (
	"strings"

	"gitee.com/hq_project/yomi-boot/errors"
)

// prepareDocuments 规范化文档并校验请求内 ID 唯一
func prepareDocuments(docs []Document) ([]Document, error) {
	if len(docs) == 0 {
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
	out := make([]Document, len(docs))
	seen := make(map[string]struct{}, len(docs))
	for i := range docs {
		doc, err := normalizeDocument(docs[i])
		if err != nil {
			return nil, err
		}
		if _, ok := seen[doc.ID]; ok {
			return nil, errors.Sentinel(ErrInvalidArgument)
		}
		seen[doc.ID] = struct{}{}
		out[i] = doc
	}
	return out, nil
}

// normalizeDocument 修剪标量字段、拷贝元数据并校验边界
func normalizeDocument(doc Document) (Document, error) {
	id := strings.TrimSpace(doc.ID)
	if err := validateDocID(id); err != nil {
		return Document{}, err
	}
	if strings.TrimSpace(doc.Content) == "" {
		return Document{}, errors.Sentinel(ErrInvalidArgument)
	}
	md := make(map[string]string, len(doc.Metadata))
	keys := make(map[string]struct{}, len(doc.Metadata))
	for rawKey, v := range doc.Metadata {
		key := strings.TrimSpace(rawKey)
		if key == "" || strings.ContainsAny(key, `"\`) || isReservedMetadataKey(key) {
			return Document{}, errors.Sentinel(ErrInvalidArgument)
		}
		// 修剪后重名会让写入值与过滤条件不一致，直接拒绝
		if _, ok := keys[key]; ok {
			return Document{}, errors.Sentinel(ErrInvalidArgument)
		}
		keys[key] = struct{}{}
		md[key] = v
	}
	if len(md) == 0 {
		md = nil
	}
	return Document{
		ID:       id,
		Content:  doc.Content,
		Source:   strings.TrimSpace(doc.Source),
		Metadata: md,
	}, nil
}

// validateDocID 校验文档 ID；块主键总长受向量库 128 字节限制
func validateDocID(id string) error {
	if id == "" || len(id) > maxDocIDLen {
		return errors.Sentinel(ErrInvalidArgument)
	}
	return nil
}

// validateFilter 校验元数据等值过滤的键
func validateFilter(filter map[string]string) error {
	for k := range filter {
		key := strings.TrimSpace(k)
		if key == "" || strings.ContainsAny(key, `"\`) {
			return errors.Sentinel(ErrInvalidArgument)
		}
	}
	return nil
}

// isReservedMetadataKey 判断是否为本包保留的元数据键
func isReservedMetadataKey(key string) bool {
	switch key {
	case MetadataKeyDocumentID, MetadataKeyChunkIndex, MetadataKeySource, MetadataKeyContent, MetadataKeyChunkCount:
		return true
	default:
		return false
	}
}
