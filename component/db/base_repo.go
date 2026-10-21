// Package db 提供通用仓储基类，封装常见 CRUD 操作
//
// @author zwei
package db

import (
	"context"
	"reflect"

	"gitee.com/hq_project/yomi-boot/boot/errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BaseRepository 通用仓储基类，可嵌入到具体仓储结构中复用
//
// 使用示例：
//
//	type UserRepo struct {
//	    db.BaseRepository
//	}
//	func NewUserRepo(d *gorm.DB) *UserRepo {
//	    return &UserRepo{BaseRepository: db.BaseRepository{DB: d}}
//	}
type BaseRepository struct {
	DB *gorm.DB
}

// idChunkSize 是批量 ID 操作的单片上限，避免触发数据库占位符上限（MySQL/PostgreSQL 均为 65535）
const idChunkSize = 500

// forEachIDChunk 按 idChunkSize 切分主键列表逐片执行，任一分片失败立即返回
func forEachIDChunk(ids []int64, fn func(chunk []int64) error) error {
	for start := 0; start < len(ids); start += idChunkSize {
		end := start + idChunkSize
		if end > len(ids) {
			end = len(ids)
		}
		if err := fn(ids[start:end]); err != nil {
			return err
		}
	}
	return nil
}

// uniqueIDs 按首次出现顺序去重，避免跨分片重复返回同一行
func uniqueIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// ============================================================================
// 写入 — Create
// ============================================================================

// Create 持久化一条记录
func (r *BaseRepository) Create(ctx context.Context, model any) error {
	if err := Exec(ctx, r.DB).Create(model).Error; err != nil {
		return errors.Wrap(err, "创建记录失败")
	}
	return nil
}

// CreateInBatches 批量持久化记录；batchSize 必须大于 0
func (r *BaseRepository) CreateInBatches(ctx context.Context, models any, batchSize int) error {
	if batchSize <= 0 {
		return errors.Newf("batchSize 必须大于 0: %d", batchSize)
	}
	if err := Exec(ctx, r.DB).CreateInBatches(models, batchSize).Error; err != nil {
		return errors.Wrap(err, "批量创建记录失败")
	}
	return nil
}

// ============================================================================
// 查询 — First / Find
// ============================================================================

// FirstByID 根据主键 id 查询单条记录
func (r *BaseRepository) FirstByID(ctx context.Context, id int64, dest any) (bool, error) {
	err := Exec(ctx, r.DB).Where("id = ?", id).First(dest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, errors.Wrap(err, "根据主键查询失败")
	}
	return true, nil
}

// FirstByField 根据指定字段查询单条记录
func (r *BaseRepository) FirstByField(ctx context.Context, field string, value any, dest any) (bool, error) {
	if err := ValidateSQLIdentifier(field); err != nil {
		return false, errors.Wrap(err, "非法字段名")
	}
	err := Exec(ctx, r.DB).Where(clause.Eq{Column: clause.Column{Name: field}, Value: value}).First(dest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, errors.Wrap(err, "根据字段查询失败")
	}
	return true, nil
}

// FindAll 查询表中全部记录
func (r *BaseRepository) FindAll(ctx context.Context, model any, dest any) error {
	if err := Exec(ctx, r.DB).Model(model).Find(dest).Error; err != nil {
		return errors.Wrap(err, "查询全部记录失败")
	}
	return nil
}

// FindByIDs 根据主键列表批量查询，dest 须为非空切片指针；超过 idChunkSize 自动分批并合并结果
func (r *BaseRepository) FindByIDs(ctx context.Context, ids []int64, model any, dest any) error {
	if len(ids) == 0 {
		return nil
	}
	v := reflect.ValueOf(dest)
	if v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Slice {
		return errors.New("FindByIDs dest 须为非空切片指针")
	}
	if len(ids) > idChunkSize {
		ids = uniqueIDs(ids)
	}

	sliceType := v.Elem().Type()
	combined := reflect.MakeSlice(sliceType, 0, len(ids))
	for start := 0; start < len(ids); start += idChunkSize {
		end := start + idChunkSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk := reflect.New(sliceType)
		if err := Exec(ctx, r.DB).Model(model).Where("id IN ?", ids[start:end]).Find(chunk.Interface()).Error; err != nil {
			return errors.Wrap(err, "根据主键列表查询失败")
		}
		combined = reflect.AppendSlice(combined, chunk.Elem())
	}
	v.Elem().Set(combined)
	return nil
}

// ExistsByField 检查指定字段值是否存在
func (r *BaseRepository) ExistsByField(ctx context.Context, field string, value any, model any) (bool, error) {
	if err := ValidateSQLIdentifier(field); err != nil {
		return false, errors.Wrap(err, "非法字段名")
	}
	var count int64
	err := Exec(ctx, r.DB).Model(model).Where(clause.Eq{Column: clause.Column{Name: field}, Value: value}).Count(&count).Error
	if err != nil {
		return false, errors.Wrap(err, "检查字段是否存在失败")
	}
	return count > 0, nil
}

// ============================================================================
// 更新 — Update
// ============================================================================

// UpdateAll 全量更新记录（按主键匹配，存在则更新全部字段，不存在则插入）
func (r *BaseRepository) UpdateAll(ctx context.Context, model any) error {
	if err := Exec(ctx, r.DB).Save(model).Error; err != nil {
		return errors.Wrap(err, "全量更新记录失败")
	}
	return nil
}

// UpdateAllBatch 批量全量更新记录（逐条按主键匹配 upsert）
func (r *BaseRepository) UpdateAllBatch(ctx context.Context, models any) error {
	if err := Exec(ctx, r.DB).Save(models).Error; err != nil {
		return errors.Wrap(err, "批量全量更新记录失败")
	}
	return nil
}

// UpdateByID 根据主键更新记录（多字段）
func (r *BaseRepository) UpdateByID(ctx context.Context, id int64, model any, updates map[string]any) error {
	if err := Exec(ctx, r.DB).Model(model).Where("id = ?", id).Updates(updates).Error; err != nil {
		return errors.Wrap(err, "根据主键更新记录失败")
	}
	return nil
}

// UpdateFieldByID 根据主键更新单个字段
func (r *BaseRepository) UpdateFieldByID(ctx context.Context, id int64, model any, field string, value any) error {
	if err := ValidateSQLIdentifier(field); err != nil {
		return errors.Wrap(err, "非法字段名")
	}
	if err := Exec(ctx, r.DB).Model(model).Where("id = ?", id).Updates(map[string]any{field: value}).Error; err != nil {
		return errors.Wrap(err, "根据主键更新字段失败")
	}
	return nil
}

// UpdateBatch 根据主键列表批量更新；超过 idChunkSize 自动分批。
// 未在事务中时，任一分片失败后已执行的分片不会回滚
func (r *BaseRepository) UpdateBatch(ctx context.Context, ids []int64, model any, updates map[string]any) error {
	if len(ids) == 0 {
		return nil
	}
	return forEachIDChunk(ids, func(chunk []int64) error {
		if err := Exec(ctx, r.DB).Model(model).Where("id IN ?", chunk).Updates(updates).Error; err != nil {
			return errors.Wrap(err, "批量更新记录失败")
		}
		return nil
	})
}

// ============================================================================
// 删除 — Delete
// ============================================================================

// DeleteByID 根据主键硬删除记录（跳过 GORM 软删除）
func (r *BaseRepository) DeleteByID(ctx context.Context, id int64, model any) error {
	if err := Exec(ctx, r.DB).Unscoped().Where("id = ?", id).Delete(model).Error; err != nil {
		return errors.Wrap(err, "根据主键删除记录失败")
	}
	return nil
}

// SoftDeleteByID 根据主键软删除记录
func (r *BaseRepository) SoftDeleteByID(ctx context.Context, id int64, model any) error {
	if err := Exec(ctx, r.DB).Model(model).Where("id = ?", id).Delete(model).Error; err != nil {
		return errors.Wrap(err, "根据主键软删除记录失败")
	}
	return nil
}

// DeleteBatch 根据主键列表批量硬删除（跳过 GORM 软删除）；超过 idChunkSize 自动分批。
// 未在事务中时，任一分片失败后已执行的分片不会回滚
func (r *BaseRepository) DeleteBatch(ctx context.Context, ids []int64, model any) error {
	if len(ids) == 0 {
		return nil
	}
	return forEachIDChunk(ids, func(chunk []int64) error {
		if err := Exec(ctx, r.DB).Unscoped().Where("id IN ?", chunk).Delete(model).Error; err != nil {
			return errors.Wrap(err, "批量删除记录失败")
		}
		return nil
	})
}

// SoftDeleteBatch 根据主键列表批量软删除；超过 idChunkSize 自动分批。
// 未在事务中时，任一分片失败后已执行的分片不会回滚
func (r *BaseRepository) SoftDeleteBatch(ctx context.Context, ids []int64, model any) error {
	if len(ids) == 0 {
		return nil
	}
	return forEachIDChunk(ids, func(chunk []int64) error {
		if err := Exec(ctx, r.DB).Model(model).Where("id IN ?", chunk).Delete(model).Error; err != nil {
			return errors.Wrap(err, "批量软删除记录失败")
		}
		return nil
	})
}

// ============================================================================
// 分页
// ============================================================================

// CountPage 执行分页查询：先 Count 再 ApplyPage 后 Find
func (r *BaseRepository) CountPage(ctx context.Context, query *gorm.DB, page, pageSize int, dest any, orders ...OrderClause) (int64, error) {
	// 调用方传入的 query 通常 clone=0，Count 会把 SQL 写入 Statement 的缓冲区，
	// 导致后续 Find 复用 "SELECT count(*)" 语句返回错误数据。
	// 先取一个 Session（clone=2），使 Count 与 Find 各自在克隆的干净 Statement 上执行。
	base := query.Session(&gorm.Session{})

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return 0, errors.Wrap(err, "分页统计失败")
	}
	if total == 0 {
		// 零命中时清空 dest，避免调用方复用切片残留上次分页结果
		if err := resetPageDest(dest); err != nil {
			return 0, errors.Wrap(err, "清空分页结果失败")
		}
		return 0, nil
	}
	paged, err := ApplyPage(base, page, pageSize, orders...)
	if err != nil {
		return 0, errors.Wrap(err, "分页排序参数非法")
	}
	if err := paged.Find(dest).Error; err != nil {
		return 0, errors.Wrap(err, "分页查询失败")
	}
	return total, nil
}

// resetPageDest 将分页 dest 置为空结果：切片置 len=0，其它可设值类型置零值
func resetPageDest(dest any) error {
	v := reflect.ValueOf(dest)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return errors.New("分页 dest 须为非空指针")
	}
	elem := v.Elem()
	if !elem.CanSet() {
		return errors.New("分页 dest 不可写")
	}
	if elem.Kind() == reflect.Slice {
		elem.Set(reflect.MakeSlice(elem.Type(), 0, 0))
		return nil
	}
	elem.Set(reflect.Zero(elem.Type()))
	return nil
}
