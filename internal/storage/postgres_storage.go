package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4/pgxpool"
	"go.uber.org/zap"
)

const (
	DatabaseOperationTimeout = 5 * time.Second
	UniqueViolationCode      = "23505"
)

type pgxStorage struct {
	ctx    context.Context
	dbConn *pgxpool.Pool
	logger zap.Logger
}

func NewDatabaseStorage(ctx context.Context, connection *pgxpool.Pool, logger *zap.Logger) (AppStorage, error) {
	if err := connection.Ping(ctx); err != nil {
		return nil, err
	}

	storage := &pgxStorage{
		ctx:    ctx,
		dbConn: connection,
		logger: *logger,
	}
	return storage, nil
}

func (p *pgxStorage) AddUser(ctx context.Context, auth *UserAuthorization) error {
	opCtx, cancel := context.WithTimeout(ctx, DatabaseOperationTimeout)
	defer cancel()

	tx, err := p.dbConn.Begin(opCtx)
	if err != nil {
		return err
	}
	defer tx.Rollback(p.ctx)

	userUUID := uuid.New()
	_, err = tx.Exec(opCtx, `INSERT INTO users (id, login, password) VALUES ($1, $2, $3);`, userUUID, auth.Login, auth.Password)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == UniqueViolationCode {
				return ErrDuplicateUser
			}
		}
		return err
	}

	_, err = tx.Exec(opCtx, `INSERT INTO balance (id, user_id, current, withdrawn) VALUES ($1, $2, 0, 0);`, uuid.New(), userUUID)
	if err != nil {
		p.logger.Sugar().Error(err)
		return err
	}

	return tx.Commit(opCtx)
}

func (p *pgxStorage) GetUserAuthInfo(ctx context.Context, userName string) (*UserAuthorization, error) {
	opCtx, cancel := context.WithTimeout(ctx, DatabaseOperationTimeout)
	defer cancel()

	r, err := p.dbConn.Query(opCtx, `SELECT id, login, password FROM users WHERE login = $1;`, userName)

	if err != nil {
		return nil, err
	}

	if err := r.Err(); err != nil {
		return nil, err
	}

	defer r.Close()

	if r.Next() {
		authData := UserAuthorization{}
		if err := r.Scan(&authData.ID, &authData.Login, &authData.Password); err != nil {
			return nil, err
		}

		return &authData, nil
	}

	return nil, ErrNoSuchUser
}

func (p *pgxStorage) GetUserAuthInfoByID(ctx context.Context, userID uuid.UUID) (*UserAuthorization, error) {
	opCtx, cancel := context.WithTimeout(ctx, DatabaseOperationTimeout)
	defer cancel()

	r, err := p.dbConn.Query(opCtx, `SELECT login, password FROM users WHERE id = $1;`, userID)

	if err != nil {
		return nil, err
	}

	if err := r.Err(); err != nil {
		return nil, err
	}

	defer r.Close()

	if r.Next() {
		authData := UserAuthorization{ID: userID}
		if err := r.Scan(&authData.Login, &authData.Password); err != nil {
			return nil, err
		}

		return &authData, nil
	}

	return nil, ErrNoSuchUser
}

func (p *pgxStorage) AddOrder(ctx context.Context, userID uuid.UUID, orderNumber string) error {
	opCtx, cancel := context.WithTimeout(ctx, DatabaseOperationTimeout)
	defer cancel()

	tx, err := p.dbConn.Begin(opCtx)
	if err != nil {
		return err
	}
	defer tx.Rollback(p.ctx)

	insertQuery := `INSERT INTO orders (id, user_id, order_number) VALUES ($1, $2, $3)`
	_, err = tx.Exec(opCtx, insertQuery, uuid.New(), userID, orderNumber)
	if err != nil {
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != UniqueViolationCode {
			return err
		}
	} else {
		return tx.Commit(opCtx)
	}

	return checkDuplicateOrder(p, opCtx, orderNumber, userID)
}

func checkDuplicateOrder(p *pgxStorage, opCtx context.Context, orderNumber string, userID uuid.UUID) error {
	query := `SELECT user_id FROM orders WHERE order_number = $1`
	r, err := p.dbConn.Query(opCtx, query, orderNumber)
	if err != nil {
		return err
	}

	if err := r.Err(); err != nil {
		return err
	}
	defer r.Close()

	if r.Next() {
		clientID := uuid.UUID{}
		if err := r.Scan(&clientID); err != nil {
			return err
		}
		if clientID == userID {
			return ErrOrderAlreadyPlaced
		}
	}
	return ErrDuplicateOrder
}

func (p *pgxStorage) UpdateOrder(ctx context.Context, order Order) error {
	opCtx, cancel := context.WithTimeout(ctx, DatabaseOperationTimeout)
	defer cancel()

	tx, err := p.dbConn.Begin(opCtx)
	if err != nil {
		return err
	}
	defer tx.Rollback(p.ctx)

	p.logger.Info("updating order", zap.Any("order_number", order.OrderNumber), zap.Float64("accrual", order.Accrual))
	_, err = tx.Exec(opCtx, `UPDATE orders SET status=$1, accrual=$2, updated_at=NOW() WHERE order_number=$3;`, order.Status, order.Accrual, order.OrderNumber)
	if err != nil {
		return err
	}

	return tx.Commit(opCtx)
}

func (p *pgxStorage) GetOrders(ctx context.Context, userID uuid.UUID) ([]Order, error) {
	opCtx, cancel := context.WithTimeout(ctx, DatabaseOperationTimeout)
	defer cancel()

	r, err := p.dbConn.Query(opCtx, `SELECT order_number, status, accrual, uploaded_at FROM orders WHERE user_id = $1;`, userID)

	if err != nil {
		return nil, err
	}

	if err := r.Err(); err != nil {
		return nil, err
	}

	defer r.Close()

	orders := make([]Order, 0)
	for r.Next() {
		order := Order{
			UserID: userID,
		}
		if err := r.Scan(&order.OrderNumber, &order.Status, &order.Accrual, &order.UploadedAt); err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}

	return orders, nil
}

func (p *pgxStorage) GetUnfinishedOrders(ctx context.Context) ([]Order, error) {
	opCtx, cancel := context.WithTimeout(ctx, DatabaseOperationTimeout)
	defer cancel()

	r, err := p.dbConn.Query(opCtx, `SELECT order_number, user_id, status, accrual, uploaded_at FROM orders WHERE status IN ('NEW', 'PROCESSING');`)

	if err != nil {
		return nil, err
	}

	if err := r.Err(); err != nil {
		return nil, err
	}

	defer r.Close()

	orders := make([]Order, 0)
	for r.Next() {
		order := Order{}
		var userID uuid.UUID
		if err := r.Scan(&order.OrderNumber, &userID, &order.Status, &order.Accrual, &order.UploadedAt); err != nil {
			return nil, err
		}
		order.UserID = userID
		orders = append(orders, order)
	}

	p.logger.Info("unfinished orders", zap.Any("orders", orders))

	return orders, nil
}

func (p *pgxStorage) Withdraw(ctx context.Context, userID uuid.UUID, order string, sum float64) error {
	opCtx, cancel := context.WithTimeout(ctx, DatabaseOperationTimeout)
	defer cancel()

	tx, err := p.dbConn.Begin(opCtx)
	if err != nil {
		return err
	}
	defer tx.Rollback(p.ctx)

	r, err := tx.Query(opCtx, `SELECT current, withdrawn FROM balance WHERE user_id = $1;`, userID)
	if err != nil {
		return err
	}

	if err := r.Err(); err != nil {
		return err
	}

	defer r.Close()

	info := BalanceInfo{}
	if r.Next() {
		if err := r.Scan(&info.Current, &info.Withdrawn); err != nil {
			return err
		}
	}

	if info.Current-sum < 0 {
		return ErrNotEnoughBalance
	}

	r.Close()

	_, err = tx.Exec(opCtx, `INSERT INTO withdrawal (id, order_number, user_id, sum) VALUES ($1, $2, $3, $4);`, uuid.New(), order, userID, sum)
	if err != nil {
		return err
	}

	_, err = tx.Exec(opCtx, `UPDATE balance SET current = current - $1, withdrawn = withdrawn + $1, updated_at = NOW() WHERE user_id = $2;`, sum, userID)
	if err != nil {
		return err
	}

	return tx.Commit(opCtx)
}

func (p *pgxStorage) AddBalance(ctx context.Context, userID uuid.UUID, amount float64) error {
	opCtx, cancel := context.WithTimeout(ctx, DatabaseOperationTimeout)
	defer cancel()

	tx, err := p.dbConn.Begin(opCtx)
	if err != nil {
		return err
	}
	defer tx.Rollback(p.ctx)

	// log
	fmt.Printf("AddBalance: %f to user %s\n", amount, userID)
	_, err = tx.Exec(opCtx, `UPDATE balance SET current = current + $1, updated_at = NOW() WHERE user_id = $2;`, amount, userID)
	if err != nil {
		return err
	}

	return tx.Commit(opCtx)
}

func (p *pgxStorage) UpdateBalanceFromOrders(ctx context.Context, orders []Order) error {
	if len(orders) == 0 {
		return nil
	}

	opCtx, cancel := context.WithTimeout(ctx, DatabaseOperationTimeout)
	defer cancel()

	tx, err := p.dbConn.Begin(opCtx)
	if err != nil {
		return err
	}
	defer tx.Rollback(p.ctx)

	totalAmount := make(map[uuid.UUID]float64)
	for _, o := range orders {
		_, err = tx.Exec(opCtx, `UPDATE orders SET status=$1, accrual=$2, updated_at=NOW() WHERE order_number=$3;`, o.Status, o.Accrual, o.OrderNumber)
		if err != nil {
			p.logger.Sugar().Errorf("UpdateOrders: %s\n", err)
			return err
		}
		totalAmount[o.UserID] += o.Accrual
	}

	for id, amount := range totalAmount {
		_, err = tx.Exec(opCtx, `UPDATE balance SET current = current + $1, updated_at = NOW() WHERE user_id = $2;`, amount, id)
		if err != nil {
			p.logger.Sugar().Errorf("UpdateBalanceFromOrders: %s\n", err)
			return err
		}
	}

	return tx.Commit(opCtx)
}

func (p *pgxStorage) GetBalance(ctx context.Context, userID uuid.UUID) (*BalanceInfo, error) {
	opCtx, cancel := context.WithTimeout(ctx, DatabaseOperationTimeout)
	defer cancel()

	r, err := p.dbConn.Query(opCtx, `SELECT current, withdrawn FROM balance WHERE user_id = $1;`, userID)

	if err != nil {
		return nil, err
	}

	if err := r.Err(); err != nil {
		return nil, err
	}

	defer r.Close()

	info := BalanceInfo{}
	if r.Next() {
		if err := r.Scan(&info.Current, &info.Withdrawn); err != nil {
			return nil, err
		}
	}

	return &info, nil
}

func (p *pgxStorage) GetWithdrawals(ctx context.Context, userID uuid.UUID) ([]Withdrawal, error) {
	opCtx, cancel := context.WithTimeout(ctx, DatabaseOperationTimeout)
	defer cancel()

	r, err := p.dbConn.Query(opCtx, `SELECT order_number, sum, processed_at FROM withdrawal WHERE user_id = $1;`, userID)

	if err != nil {
		p.logger.Sugar().Errorf("GetWithdrawals: %s\n", err)
		return nil, err
	}

	if err := r.Err(); err != nil {
		p.logger.Sugar().Errorf("GetWithdrawals: %s\n", err)
		return nil, err
	}

	defer r.Close()

	ws := make([]Withdrawal, 0)
	for r.Next() {
		w := Withdrawal{}
		if err := r.Scan(&w.OrderNumber, &w.Sum, &w.ProcessedAt); err != nil {
			p.logger.Sugar().Errorf("GetWithdrawals: %s\n", err)
			return nil, err
		}
		ws = append(ws, w)
	}

	p.logger.Sugar().Infof("GetWithdrawals: %v", ws)

	return ws, nil
}
