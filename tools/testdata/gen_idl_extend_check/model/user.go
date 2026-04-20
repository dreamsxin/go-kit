package model

import "gorm.io/gorm"

// BeforeCreate runs before inserting User.
func (m *User) BeforeCreate(tx *gorm.DB) (err error) {
	// TODO: Add create-time model hooks here.
	return nil
}

// BeforeUpdate runs before updating User.
func (m *User) BeforeUpdate(tx *gorm.DB) (err error) {
	// TODO: Add update-time model hooks here.
	return nil
}
