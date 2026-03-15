package multi

import "context"

type OrderItem struct {
	ProductID uint    `json:"product_id" gorm:"primaryKey"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}

type PlaceOrderRequest struct {
	UserID uint `json:"user_id"`
}
type PlaceOrderResponse struct {
	OrderID uint   `json:"order_id"`
	Error   string `json:"error"`
}

type GetOrderRequest struct {
	ID uint `json:"id"`
}
type GetOrderResponse struct {
	Items []*OrderItem `json:"items"`
	Error string       `json:"error"`
}

// OrderService handles orders.
type OrderService interface {
	PlaceOrder(ctx context.Context, req PlaceOrderRequest) (PlaceOrderResponse, error)
	GetOrder(ctx context.Context, req GetOrderRequest) (GetOrderResponse, error)
}

type ProductModel struct {
	ID    uint    `json:"id"    gorm:"primaryKey;autoIncrement"`
	Name  string  `json:"name"  gorm:"not null"`
	Price float64 `json:"price"`
}

type IncrStockRequest struct {
	ProductID uint `json:"product_id"`
}
type IncrStockResponse struct {
	Stock int    `json:"stock"`
	Error string `json:"error"`
}

// ProductService handles products.
type ProductService interface {
	IncrStock(ctx context.Context, req IncrStockRequest) (IncrStockResponse, error)
}
