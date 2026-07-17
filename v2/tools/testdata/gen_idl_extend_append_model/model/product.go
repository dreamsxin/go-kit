package model

import "gorm.io/gorm"

// BeforeCreate runs before inserting Product.
func (m *Product) BeforeCreate(tx *gorm.DB) (err error) {
	// TODO: Add create-time model hooks here.
	return nil
}

// BeforeUpdate runs before updating Product.
func (m *Product) BeforeUpdate(tx *gorm.DB) (err error) {
	// TODO: Add update-time model hooks here.
	return nil
}
