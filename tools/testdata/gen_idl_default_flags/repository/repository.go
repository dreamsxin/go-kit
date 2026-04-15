package repository


import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"example.com/gen_idl_default_flags/model"
)


// ─────────────────────────── User Repository ───────────────────────────

// UserRepository User 数据访问层
type UserRepository struct {
	db *DB
}

// NewUserRepository 创建 UserRepository
func NewUserRepository(db *DB) *UserRepository {
	return &UserRepository{db: db}
}

// Create 插入新记录（自增 ID 写回 m.ID）
func (r *UserRepository) Create(ctx context.Context, m *model.User) error {
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return fmt.Errorf("user.Create: %w", err)
	}
	return nil
}

// GetByID 按主键查询，不存在时返回 (nil, nil)
func (r *UserRepository) GetByID(ctx context.Context, id uint) (*model.User, error) {
	var m model.User
	err := r.db.WithContext(ctx).First(&m, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("user.GetByID(%d): %w", id, err)
	}
	return &m, nil
}

// Update 全量保存（Save 会更新所有字段，含零值）
func (r *UserRepository) Update(ctx context.Context, m *model.User) error {
	if err := r.db.WithContext(ctx).Save(m).Error; err != nil {
		return fmt.Errorf("user.Update: %w", err)
	}
	return nil
}

// Updates 部分更新（只更新 values map 中的字段）
func (r *UserRepository) Updates(ctx context.Context, id uint, values map[string]interface{}) error {
	result := r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).Updates(values)
	if result.Error != nil {
		return fmt.Errorf("user.Updates(%d): %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("user.Updates(%d): %w", id, ErrNotFound)
	}
	return nil
}

// Delete 软删除（model 含 gorm.DeletedAt 时为软删；不含则硬删）
func (r *UserRepository) Delete(ctx context.Context, id uint) error {
	result := r.db.WithContext(ctx).Delete(&model.User{}, id)
	if result.Error != nil {
		return fmt.Errorf("user.Delete(%d): %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("user.Delete(%d): %w", id, ErrNotFound)
	}
	return nil
}

// UserListResult 分页查询结果
type UserListResult struct {
	Total    int64          `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
	Data     []model.User `json:"data"`
}

// List 分页查询（支持关键词搜索与排序）
//
// 搜索字段可在此扩展，如：
//   db = db.Where("username LIKE ? OR email LIKE ?", kw, kw)
func (r *UserRepository) List(ctx context.Context, q PageQuery) (*UserListResult, error) {
	q.normalize()

	db := r.db.WithContext(ctx).Model(&model.User{})

	// 关键词搜索（可按实际字段扩展）
	if q.Keyword != "" {
		kw := "%" + q.Keyword + "%"
		// TODO: 根据实际字段调整搜索条件，例如：
		// db = db.Where("name LIKE ? OR description LIKE ?", kw, kw)
		_ = kw
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("user.List count: %w", err)
	}

	// 排序
	order := q.OrderBy
	if q.Desc {
		order += " DESC"
	} else {
		order += " ASC"
	}
	db = db.Order(order)

	// 分页
	offset := (q.Page - 1) * q.PageSize
	db = db.Offset(offset).Limit(q.PageSize)

	var records []model.User
	if err := db.Find(&records).Error; err != nil {
		return nil, fmt.Errorf("user.List query: %w", err)
	}

	return &UserListResult{
		Total:    total,
		Page:     q.Page,
		PageSize: q.PageSize,
		Data:     records,
	}, nil
}


