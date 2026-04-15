package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// ─────────────────────────── 通用类型 ───────────────────────────

// PageQuery 分页 + 排序查询参数
type PageQuery struct {
	Page     int    `json:"page"      form:"page"`       // 页码，从 1 开始，默认 1
	PageSize int    `json:"page_size" form:"page_size"`  // 每页条数，默认 20，最大 200
	OrderBy  string `json:"order_by"  form:"order_by"`   // 排序字段（列名），默认 id
	Desc     bool   `json:"desc"      form:"desc"`        // true 降序，false 升序
	Keyword  string `json:"keyword"   form:"keyword"`    // 模糊搜索关键词（由各 repository 决定搜索范围）
}

// normalize 将 PageQuery 中的默认值补全
func (q *PageQuery) normalize() {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 {
		q.PageSize = 20
	}
	if q.PageSize > 200 {
		q.PageSize = 200
	}
	if q.OrderBy == "" {
		q.OrderBy = "id"
	}
}

// ─────────────────────────── DB 包装 ───────────────────────────

// DB 包装 gorm.DB，统一管理连接
type DB struct {
	db *gorm.DB
}

// NewDB 创建 DB 实例
func NewDB(db *gorm.DB) *DB {
	return &DB{db: db}
}

// WithContext 获取带 context 的 gorm.DB
func (d *DB) WithContext(ctx context.Context) *gorm.DB {
	return d.db.WithContext(ctx)
}

// AutoMigrate 自动迁移所有 model（在 main.go 中调用）
func (d *DB) AutoMigrate(dst ...interface{}) error {
	return d.db.AutoMigrate(dst...)
}

// ErrNotFound 记录未找到错误（可在业务层做类型断言）
var ErrNotFound = errors.New("record not found")
