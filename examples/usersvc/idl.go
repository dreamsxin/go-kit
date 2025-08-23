package usersvc

import "context"

// User 表示用户实体数据结构
type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Age      int    `json:"age"`
}

// CreateUserRequest 定义创建用户的请求参数
type CreateUserRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Age      int    `json:"age"`
}

// CreateUserResponse 定义创建用户的响应结果
type CreateUserResponse struct {
	User  *User  `json:"user,omitempty"`
	Error string `json:"error,omitempty"`
}

// GetUserRequest 定义获取用户的请求参数
type GetUserRequest struct {
	ID string `json:"id"`
}

// GetUserResponse 定义获取用户的响应结果
type GetUserResponse struct {
	User  *User  `json:"user,omitempty"`
	Error string `json:"error,omitempty"`
}

// UpdateUserRequest 定义更新用户的请求参数
type UpdateUserRequest struct {
	ID       string `json:"id"`
	Username string `json:"username,omitempty"`
	Email    string `json:"email,omitempty"`
	Age      int    `json:"age,omitempty"`
}

// UpdateUserResponse 定义更新用户的响应结果
type UpdateUserResponse struct {
	User  *User  `json:"user,omitempty"`
	Error string `json:"error,omitempty"`
}

// DeleteUserRequest 定义删除用户的请求参数
type DeleteUserRequest struct {
	ID string `json:"id"`
}

// DeleteUserResponse 定义删除用户的响应结果
type DeleteUserResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// UserService 定义用户服务的核心接口
type UserService interface {
	// 创建用户
	CreateUser(ctx context.Context, req CreateUserRequest) (CreateUserResponse, error)

	// 获取用户
	GetUser(ctx context.Context, req GetUserRequest) (GetUserResponse, error)

	// 更新用户
	UpdateUser(ctx context.Context, req UpdateUserRequest) (UpdateUserResponse, error)

	// 删除用户
	DeleteUser(ctx context.Context, req DeleteUserRequest) (DeleteUserResponse, error)
}

// ManageService 定义用户服务的核心接口
type ManageService interface {
	// 创建用户
	CreateUser(ctx context.Context, req CreateUserRequest) (CreateUserResponse, error)

	// 获取用户
	GetUser(ctx context.Context, req GetUserRequest) (GetUserResponse, error)

	// 更新用户
	UpdateUser(ctx context.Context, req UpdateUserRequest) (UpdateUserResponse, error)

	// 删除用户
	DeleteUser(ctx context.Context, req DeleteUserRequest) (DeleteUserResponse, error)
}
