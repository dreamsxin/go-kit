package multisvc

import "context"

// ─────────────────────────── UserService ───────────────────────────

type CreateUserRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
}
type CreateUserResponse struct {
	ID    uint   `json:"id"`
	Error string `json:"error,omitempty"`
}
type GetUserRequest struct {
	ID uint `json:"id"`
}
type GetUserResponse struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Error    string `json:"error,omitempty"`
}

// UserService 用户服务
type UserService interface {
	// CreateUser 创建用户
	CreateUser(ctx context.Context, req CreateUserRequest) (CreateUserResponse, error)
	// GetUser 获取用户
	GetUser(ctx context.Context, req GetUserRequest) (GetUserResponse, error)
}

// ─────────────────────────── OrderService ───────────────────────────

type CreateOrderRequest struct {
	UserID uint   `json:"user_id"`
	Item   string `json:"item"`
}
type CreateOrderResponse struct {
	OrderID uint   `json:"order_id"`
	Error   string `json:"error,omitempty"`
}
type GetOrderRequest struct {
	OrderID uint `json:"order_id"`
}
type GetOrderResponse struct {
	OrderID uint   `json:"order_id"`
	Item    string `json:"item"`
	Error   string `json:"error,omitempty"`
}

// OrderService 订单服务
type OrderService interface {
	// CreateOrder 创建订单
	CreateOrder(ctx context.Context, req CreateOrderRequest) (CreateOrderResponse, error)
	// GetOrder 获取订单
	GetOrder(ctx context.Context, req GetOrderRequest) (GetOrderResponse, error)
}
