package storage

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

const (
	StatusNew        = "NEW"
	StatusInvalid    = "INVALID"
	StatusProcessing = "PROCESSING"
	StatusProcessed  = "PROCESSED"
)

var (
	ErrDuplicateUser      = errors.New("duplicate user")
	ErrNoSuchUser         = errors.New("no such user")
	ErrNotEnoughBalance   = errors.New("not enough balance")
	ErrDuplicateOrder     = errors.New("duplicate order")
	ErrOrderAlreadyPlaced = errors.New("order already placed")
)

type UserAuthorization struct {
	ID        uuid.UUID `json:"id"`
	Login     string    `json:"login"`
	Password  []byte    `json:"password"`
	CreatedAt time.Time `json:"created_at"`
}

type BalanceInfo struct {
	Current   float64   `json:"current"`
	Withdrawn float64   `json:"withdrawn"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Withdrawal struct {
	OrderNumber string    `json:"order"`
	UserID      uuid.UUID `json:"user_id"`
	Sum         float64   `json:"sum"`
	ProcessedAt time.Time `json:"processed_at"`
}

type Order struct {
	UserID      uuid.UUID `json:"user_id"`
	OrderNumber string    `json:"order_number"`
	Status      string    `json:"status"`
	Accrual     float64   `json:"accrual"`
	UploadedAt  time.Time `json:"uploaded_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type AppStorage interface {
	AddUser(ctx context.Context, auth *UserAuthorization) error
	GetUserAuthInfo(ctx context.Context, userName string) (*UserAuthorization, error)
	GetUserAuthInfoByID(ctx context.Context, userID uuid.UUID) (*UserAuthorization, error)

	Withdraw(ctx context.Context, userID uuid.UUID, order string, sum float64) error
	AddBalance(ctx context.Context, userID uuid.UUID, amount float64) error
	UpdateBalanceFromOrders(ctx context.Context, orders []Order) error
	GetBalance(ctx context.Context, userID uuid.UUID) (*BalanceInfo, error)
	GetWithdrawals(ctx context.Context, userID uuid.UUID) ([]Withdrawal, error)

	AddOrder(ctx context.Context, userID uuid.UUID, orderNumber string) error
	UpdateOrder(ctx context.Context, order Order) error
	GetOrders(ctx context.Context, userID uuid.UUID) ([]Order, error)
	GetUnfinishedOrders(ctx context.Context) ([]Order, error)
}
