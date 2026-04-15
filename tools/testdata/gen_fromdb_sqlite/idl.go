package gen

import (
	"context"
)

// ─────────────────────────── GORM Models ───────────────────────────

// User 对应数据库表 users
type User struct {
	ID int `json:"id" gorm:"column:id;primaryKey;autoIncrement;type:INTEGER"`
	Username string `json:"username" gorm:"column:username;not null;type:TEXT"`
	Email string `json:"email" gorm:"column:email;not null;type:TEXT"`
}

// ─────────────────────────── DTOs ───────────────────────────

// UserItem 对外暴露的 User 实体
type UserItem struct {
	ID int `json:"id"`
	Username string `json:"username"`
	Email string `json:"email"`
}

// CreateUserRequest 创建请求
type CreateUserRequest struct {
	Username string `json:"username"`
	Email string `json:"email"`
}

// CreateUserResponse 创建响应
type CreateUserResponse struct {
	Data  *UserItem `json:"data,omitempty"`
	Error string    `json:"error,omitempty"`
}

// GetUserRequest 获取请求
type GetUserRequest struct {
	ID uint `json:"id"`
}

// GetUserResponse 获取响应
type GetUserResponse struct {
	Data  *UserItem `json:"data,omitempty"`
	Error string    `json:"error,omitempty"`
}

// UpdateUserRequest 更新请求
type UpdateUserRequest struct {
	ID uint `json:"id"`
	Username *string `json:"username,omitempty"`
	Email *string `json:"email,omitempty"`
}

// UpdateUserResponse 更新响应
type UpdateUserResponse struct {
	Data  *UserItem `json:"data,omitempty"`
	Error string    `json:"error,omitempty"`
}

// DeleteUserRequest 删除请求
type DeleteUserRequest struct {
	ID uint `json:"id"`
}

// DeleteUserResponse 删除响应
type DeleteUserResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// ListUsersRequest 列表查询请求
type ListUsersRequest struct {
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Keyword  string `json:"keyword,omitempty"`
}

// ListUsersResponse 列表查询响应
type ListUsersResponse struct {
	Data     []UserItem `json:"data"`
	Total    int64      `json:"total"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
	Error    string     `json:"error,omitempty"`
}

// ─────────────────────────── Service 接口 ───────────────────────────

// GenService 自动生成的 RESTful 服务接口
type GenService interface {
	// CreateUser 创建
	CreateUser(ctx context.Context, req CreateUserRequest) (CreateUserResponse, error)

	// GetUser 获取详情
	GetUser(ctx context.Context, req GetUserRequest) (GetUserResponse, error)

	// UpdateUser 更新
	UpdateUser(ctx context.Context, req UpdateUserRequest) (UpdateUserResponse, error)

	// DeleteUser 删除
	DeleteUser(ctx context.Context, req DeleteUserRequest) (DeleteUserResponse, error)

	// ListUsers 分页查询
	ListUsers(ctx context.Context, req ListUsersRequest) (ListUsersResponse, error)

}
