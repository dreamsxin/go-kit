package model

import (
	"time"

	"gorm.io/gorm"
)


// User
type User struct {
	ID uint `json:"id" gorm:"primaryKey;autoIncrement"`
	Username string `json:"username" gorm:"column:username;not null;uniqueIndex"`
	Email string `json:"email" gorm:"column:email;not null"`
	Age int `json:"age"`
	Score float64 `json:"score"`
	Active bool `json:"active"`
	CreatedAt time.Time      `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time      `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`
}

// TableName 指定表名（gorm 约定）
func (User) TableName() string {
	return "user"
}


