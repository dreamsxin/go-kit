package usersvc

import "context"

// ─────────────────────────── GORM Model ───────────────────────────

// User 用户数据库模型（带 gorm tag，microgen -model 时生成 model/repository 代码）
type User struct {
	ID       uint   `json:"id"       gorm:"primaryKey;autoIncrement"`
	Username string `json:"username" gorm:"column:username;type:varchar(64);not null;uniqueIndex"` // 用户名（唯一）
	Email    string `json:"email"    gorm:"column:email;type:varchar(128);not null;uniqueIndex"`   // 邮箱（唯一）
	Age      int    `json:"age"      gorm:"column:age;type:tinyint unsigned"`                      // 年龄
	Status   int    `json:"status"   gorm:"column:status;type:tinyint;default:1"`                  // 状态 1=正常 0=禁用
}

// ─────────────────────────── DTO ───────────────────────────

// CreateUserRequest 创建用户请求
type CreateUserRequest struct {
	Username string `json:"username"` // 用户名，必填
	Email    string `json:"email"`    // 邮箱，必填
	Age      int    `json:"age"`      // 年龄
}

// CreateUserResponse 创建用户响应
type CreateUserResponse struct {
	User  *User  `json:"user,omitempty"`
	Error string `json:"error,omitempty"`
}

// GetUserRequest 获取用户请求
type GetUserRequest struct {
	ID uint `json:"id"`
}

// GetUserResponse 获取用户响应
type GetUserResponse struct {
	User  *User  `json:"user,omitempty"`
	Error string `json:"error,omitempty"`
}

// UpdateUserRequest 更新用户请求
type UpdateUserRequest struct {
	ID       uint   `json:"id"`
	Username string `json:"username,omitempty"`
	Email    string `json:"email,omitempty"`
	Age      int    `json:"age,omitempty"`
}

// UpdateUserResponse 更新用户响应
type UpdateUserResponse struct {
	User  *User  `json:"user,omitempty"`
	Error string `json:"error,omitempty"`
}

// DeleteUserRequest 删除用户请求
type DeleteUserRequest struct {
	ID uint `json:"id"`
}

// DeleteUserResponse 删除用户响应
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
	Total int     `json:"total"`
	Users []*User `json:"users"`
	Error string  `json:"error,omitempty"`
}

// ─────────────────────────── Service 接口 ───────────────────────────

// UserService 用户服务接口
type UserService interface {
	// CreateUser 创建用户
	CreateUser(ctx context.Context, req CreateUserRequest) (CreateUserResponse, error)

	// GetUser 获取用户详情
	GetUser(ctx context.Context, req GetUserRequest) (GetUserResponse, error)

	// UpdateUser 更新用户信息
	UpdateUser(ctx context.Context, req UpdateUserRequest) (UpdateUserResponse, error)

	// DeleteUser 删除用户
	DeleteUser(ctx context.Context, req DeleteUserRequest) (DeleteUserResponse, error)

	// ListUsers 分页查询用户列表
	ListUsers(ctx context.Context, req ListUsersRequest) (ListUsersResponse, error)
}
