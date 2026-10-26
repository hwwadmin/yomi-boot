// 检索后置过滤器契约。
//
// @author zwei

package retrieval

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/ai/vectordb"
)

// HitFilter 检索后置过滤器：对阈值过滤后的命中叠加域语义过滤（如文档版本治理、
// 内容完整性），返回保留的命中；实现须并发安全，不得修改入参。
//
// 返回 vectordb.ErrNotFound 等同于集合消失，检索按空结果处理；
// 其他错误原样上抛，调用方可判断其哨兵。
type HitFilter interface {
	Filter(ctx context.Context, collection string, hits []vectordb.Hit) ([]vectordb.Hit, error)
}
