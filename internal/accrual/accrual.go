package accrual

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"go.uber.org/zap"

	"github.com/real-splendid/gophermart-practicum/internal/storage"
)

const (
	StatusRegistered = "REGISTERED"
	StatusInvalid    = "INVALID"
	StatusProcessing = "PROCESSING"
	StatusProcessed  = "PROCESSED"
)

type orderInfo struct {
	Order   string  `json:"order"`
	Status  string  `json:"status"`
	Accrual float64 `json:"accrual"`
}

type Config struct {
	BaseAddr string
	Logger   *zap.Logger
	storage.AppStorage
}

type Accrual struct {
	ctx       context.Context
	ctxCancel context.CancelFunc
	client    *resty.Client
	Config
}

func NewAccrual(ctx context.Context, cfg Config) *Accrual {
	ctx, cancel := context.WithCancel(ctx)

	retryFn := resty.RetryAfterFunc(func(client *resty.Client, response *resty.Response) (time.Duration, error) {
		if response.StatusCode() != http.StatusTooManyRequests {
			return 0, nil
		}

		retryAfterValue := response.Header().Get("Retry-After")
		if len(retryAfterValue) == 0 {
			return 0, nil
		}

		seconds, err := strconv.ParseInt(retryAfterValue, 10, 64)
		if err != nil {
			return 0, err
		}

		return time.Duration(seconds) * time.Second, nil
	})

	client := resty.New().SetRetryAfter(retryFn).SetRetryCount(3)

	updater := &Accrual{
		ctx:       ctx,
		ctxCancel: cancel,
		client:    client,
		Config:    cfg,
	}

	go updater.updateOrders()

	return updater
}

func (u *Accrual) Stop() {
	u.ctxCancel()
}

func (u *Accrual) updateOrders() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			u.update()
		case <-u.ctx.Done():
			return

		}
	}
}

func (u *Accrual) update() {
	orders, err := u.GetUnfinishedOrders(u.ctx)
	if err != nil {
		return
	}
	if len(orders) == 0 {
		return
	}

	var wg sync.WaitGroup
	ordersInfo := make([]*orderInfo, len(orders))

	ordersWithBalanceUpdate := make([]storage.Order, 0)
	for i, o := range orders {
		wg.Add(1)
		go func(index int, o storage.Order) {
			defer wg.Done()
			info, err := u.getOrderStatus(o.OrderNumber)
			if err != nil {
				return
			}
			ordersInfo[index] = info
		}(i, o)
	}

	wg.Wait()

	for i, info := range ordersInfo {
		if info == nil {
			continue
		}

		switch info.Status {
		case StatusRegistered, StatusProcessing:
			orders[i].Status = storage.StatusProcessing
		case StatusInvalid:
			orders[i].Status = storage.StatusInvalid
		case StatusProcessed:
			orders[i].Status = storage.StatusProcessed
			orders[i].Accrual = info.Accrual
		}

		if info.Status == storage.StatusProcessed {
			ordersWithBalanceUpdate = append(ordersWithBalanceUpdate, orders[i])
		}

		if err := u.UpdateOrder(u.ctx, orders[i]); err != nil {
			u.Logger.Error("can't update order", zap.Error(err))
		}
	}

	if err := u.UpdateBalanceFromOrders(u.ctx, ordersWithBalanceUpdate); err != nil {
		u.Logger.Error("can't update balance", zap.Error(err))
	}
}

func (u *Accrual) getOrderStatus(orderID string) (*orderInfo, error) {
	request := u.client.R().SetContext(u.ctx)

	url := fmt.Sprintf("%s/api/orders/%s", u.BaseAddr, orderID)
	response, err := request.Get(url)
	if err != nil {
		return nil, err
	}

	if response.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("bad status code: %d", response.StatusCode())
	}

	var info orderInfo
	if err := json.Unmarshal(response.Body(), &info); err != nil {
		return nil, err
	}

	return &info, nil
}
