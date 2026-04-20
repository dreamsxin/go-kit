package basic

import "context"

// User is a simple DTO.
type User struct {
	ID       uint   `json:"id"       gorm:"primaryKey;autoIncrement"`
	Username string `json:"username" gorm:"column:username;not null;uniqueIndex"`
	Email    string `json:"email"    gorm:"column:email;not null"`
	Age      int    `json:"age"`
	Score    float64 `json:"score"`
	Active   bool   `json:"active"`
}

type CreateUserRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
}

type CreateUserResponse struct {
	User  *User  `json:"user"`
	Error string `json:"error"`
}

type GetUserRequest struct {
	ID uint `json:"id"`
}

type GetUserResponse struct {
	User  *User  `json:"user"`
	Error string `json:"error"`
}

type ListUsersRequest struct {
	Page    int    `json:"page"`
	Keyword string `json:"keyword"`
}

type ListUsersResponse struct {
	Users []*User `json:"users"`
	Total int     `json:"total"`
}

type DeleteUserRequest struct {
	ID uint `json:"id"`
}

type DeleteUserResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

type UpdateUserRequest struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
}

type UpdateUserResponse struct {
	User  *User  `json:"user"`
	Error string `json:"error"`
}

// UserService manages users.
type UserService interface {
	// CreateUser creates a new user.
	CreateUser(ctx context.Context, req CreateUserRequest) (CreateUserResponse, error)
	// GetUser retrieves a user by ID.
	GetUser(ctx context.Context, req GetUserRequest) (GetUserResponse, error)
	// ListUsers lists all users.
	ListUsers(ctx context.Context, req ListUsersRequest) (ListUsersResponse, error)
	// DeleteUser removes a user.
	DeleteUser(ctx context.Context, req DeleteUserRequest) (DeleteUserResponse, error)
	// UpdateUser modifies a user.
	UpdateUser(ctx context.Context, req UpdateUserRequest) (UpdateUserResponse, error)
	// FindByEmail finds users by email prefix.
	FindByEmail(ctx context.Context, req GetUserRequest) (GetUserResponse, error)
	// SearchUsers searches users.
	SearchUsers(ctx context.Context, req ListUsersRequest) (ListUsersResponse, error)
	// QueryStats returns statistics.
	QueryStats(ctx context.Context, req GetUserRequest) (GetUserResponse, error)
	// RemoveExpired removes expired users.
	RemoveExpired(ctx context.Context, req DeleteUserRequest) (DeleteUserResponse, error)
	// EditProfile edits profile.
	EditProfile(ctx context.Context, req UpdateUserRequest) (UpdateUserResponse, error)
	// ModifyEmail modifies email.
	ModifyEmail(ctx context.Context, req UpdateUserRequest) (UpdateUserResponse, error)
	// PatchStatus patches status.
	PatchStatus(ctx context.Context, req UpdateUserRequest) (UpdateUserResponse, error)
}


type PlaceOrderRequest struct {
	UserID uint `json:"user_id"`
}
type PlaceOrderResponse struct {
	OrderID uint   `json:"order_id"`
	Error   string `json:"error"`
}

type OrderService interface {
	PlaceOrder(ctx context.Context, req PlaceOrderRequest) (PlaceOrderResponse, error)
}
