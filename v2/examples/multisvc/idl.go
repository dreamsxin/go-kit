package multisvc

import "context"

// ─────────────────────────── GORM Models ───────────────────────────

// User 用户数据库模型
type User struct {
	ID       uint   `json:"id"       gorm:"primaryKey;autoIncrement"`
	Username string `json:"username" gorm:"column:username;type:varchar(64);not null;uniqueIndex"`
	Email    string `json:"email"    gorm:"column:email;type:varchar(128);not null;uniqueIndex"`
}

// Order 订单数据库模型
type Order struct {
	ID     uint   `json:"id"      gorm:"primaryKey;autoIncrement"`
	UserID uint   `json:"user_id" gorm:"column:user_id;not null;index"`
	Item   string `json:"item"    gorm:"column:item;type:varchar(256);not null"`
}

// ─────────────────────────── UserService DTO ───────────────────────────

// CreateUserRequest 创建用户请求
type CreateUserRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
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

// ─────────────────────────── OrderService DTO ───────────────────────────

// CreateOrderRequest 创建订单请求
type CreateOrderRequest struct {
	UserID uint   `json:"user_id"`
	Item   string `json:"item"`
}

// CreateOrderResponse 创建订单响应
type CreateOrderResponse struct {
	Order *Order `json:"order,omitempty"`
	Error string `json:"error,omitempty"`
}

// GetOrderRequest 获取订单请求
type GetOrderRequest struct {
	OrderID uint `json:"order_id"`
}

// GetOrderResponse 获取订单响应
type GetOrderResponse struct {
	Order *Order `json:"order,omitempty"`
	Error string `json:"error,omitempty"`
}

// ─────────────────────────── Service 接口 ───────────────────────────

// UserService 用户服务
type UserService interface {
	// CreateUser 创建用户
	CreateUser(ctx context.Context, req CreateUserRequest) (CreateUserResponse, error)
	// GetUser 获取用户
	GetUser(ctx context.Context, req GetUserRequest) (GetUserResponse, error)
}

// OrderService 订单服务
type OrderService interface {
	// CreateOrder 创建订单
	CreateOrder(ctx context.Context, req CreateOrderRequest) (CreateOrderResponse, error)
	// GetOrder 获取订单
	GetOrder(ctx context.Context, req GetOrderRequest) (GetOrderResponse, error)
}
