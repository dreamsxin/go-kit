package model

import "gorm.io/gorm"

// ─────────────────────────── User Hooks ──────────────────────────────────────────

// BeforeCreate 在插入记录前执行
func (m *User) BeforeCreate(tx *gorm.DB) (err error) {
	// TODO: 在此添加创建前的逻辑（如生成 UUID）
	return nil
}

// BeforeUpdate 在更新记录前执行
func (m *User) BeforeUpdate(tx *gorm.DB) (err error) {
	// TODO: 在此添加更新前的逻辑
	return nil
}
