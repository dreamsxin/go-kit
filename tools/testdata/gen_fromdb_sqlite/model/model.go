package model

import (
	"time"

	"gorm.io/gorm"
)


// User
type User struct {
	ID int `json:"id" gorm:"column:id;primaryKey;autoIncrement;type:INTEGER"`
	Username string `json:"username" gorm:"column:username;not null;type:TEXT"`
	Email string `json:"email" gorm:"column:email;not null;type:TEXT"`
	CreatedAt time.Time      `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time      `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`
}

// TableName 指定表名（gorm 约定）
func (User) TableName() string {
	return "users"
}


