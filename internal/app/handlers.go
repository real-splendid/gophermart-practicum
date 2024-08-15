package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/real-splendid/gophermart-practicum/internal/storage"
)

type HandlersServer struct {
	ctx            context.Context
	logger         *zap.Logger
	storageService storage.AppStorage
}

type orderResponse struct {
	Number     string    `json:"number"`
	Status     string    `json:"status"`
	Accrual    float64   `json:"accrual,omitempty"`
	UploadedAt time.Time `json:"uploaded_at"`
}

type withdrawalsResponse struct {
	Order       string    `json:"order"`
	Sum         float64   `json:"sum"`
	ProcessedAt time.Time `json:"processed_at"`
}

type balanceWithdrawRequest struct {
	Order string  `json:"order"`
	Sum   float64 `json:"sum"`
}

func NewHandlersServer(ctx context.Context, logger *zap.Logger, storage storage.AppStorage) (*HandlersServer, error) {
	server := &HandlersServer{
		ctx:            ctx,
		logger:         logger,
		storageService: storage,
	}

	return server, nil
}

func (s *HandlersServer) apiAddUserOrder(w http.ResponseWriter, r *http.Request) {
	if contentType := r.Header.Get("Content-Type"); contentType != "text/plain" {
		s.logger.Error("bad content type", zap.String("content_type", contentType))
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	b, err := io.ReadAll(r.Body)
	if err != nil {
		s.logger.Error("failed to read request body", zap.Error(err))
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	if !isCorrectOrderNum(string(b)) {
		s.logger.Info("bad order id", zap.String("order_id", string(b)))
		http.Error(w, "", http.StatusUnprocessableEntity)
		return
	}

	orderID := string(b)
	userData := r.Context().Value(UserAuthDataCtxKey).(*storage.UserAuthorization)

	if err := s.storageService.AddOrder(r.Context(), userData.ID, orderID); err != nil {
		if errors.Is(err, storage.ErrDuplicateOrder) {
			s.logger.Error("duplicate order id", zap.String("order_id", orderID))
			http.Error(w, "", http.StatusConflict)
			return
		}
		if errors.Is(err, storage.ErrOrderAlreadyPlaced) {
			s.logger.Info("order already placed", zap.String("order_id", orderID))
			w.WriteHeader(http.StatusOK)
			return
		}
		s.logger.Error("failed to add order", zap.Error(err))
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (s *HandlersServer) apiGetUserOrders(w http.ResponseWriter, r *http.Request) {
	userData := r.Context().Value(UserAuthDataCtxKey).(*storage.UserAuthorization)

	orders, err := s.storageService.GetOrders(r.Context(), userData.ID)
	if err != nil {
		s.logger.Error("get orders failed", zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	respData := make([]orderResponse, len(orders))
	for i, e := range orders {
		respData[i] = orderResponse{
			Number:     e.OrderNumber,
			Status:     e.Status,
			Accrual:    e.Accrual,
			UploadedAt: e.UploadedAt,
		}
	}

	s.apiWriteResponse(w, http.StatusOK, respData)
}

func (s *HandlersServer) apiGetUserWithdrawals(w http.ResponseWriter, r *http.Request) {
	userData := r.Context().Value(UserAuthDataCtxKey).(*storage.UserAuthorization)

	ws, err := s.storageService.GetWithdrawals(r.Context(), userData.ID)
	if err != nil {
		s.logger.Error("failed to get withdrawals", zap.String("user_id", userData.ID.String()), zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	if len(ws) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	responseData := make([]withdrawalsResponse, len(ws))
	for i, e := range ws {
		responseData[i] = withdrawalsResponse{
			Order:       e.OrderNumber,
			Sum:         e.Sum,
			ProcessedAt: e.ProcessedAt,
		}
	}

	s.apiWriteResponse(w, http.StatusOK, responseData)
}

func (s *HandlersServer) apiGetUserBalance(w http.ResponseWriter, r *http.Request) {
	userData := r.Context().Value(UserAuthDataCtxKey).(*storage.UserAuthorization)

	balance, err := s.storageService.GetBalance(r.Context(), userData.ID)
	s.logger.Info("got balance", zap.String("user_id", userData.ID.String()), zap.Any("balance", balance))
	if err != nil {
		s.logger.Error("failed to get balance", zap.String("user_id", userData.ID.String()), zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	s.apiWriteResponse(w, http.StatusOK, balance)
}

func (s *HandlersServer) apiBalanceWithdraw(w http.ResponseWriter, r *http.Request) {
	userData := r.Context().Value(UserAuthDataCtxKey).(*storage.UserAuthorization)

	withdrawRequest := balanceWithdrawRequest{}
	if err := s.apiParseRequest(r, &withdrawRequest); err != nil {
		s.logger.Error("failed to withdraw balance", zap.String("user_id", userData.ID.String()), zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	if !isCorrectOrderNum(withdrawRequest.Order) {
		s.logger.Error("bad order id", zap.String("order_id", withdrawRequest.Order))
		http.Error(w, "", http.StatusUnprocessableEntity)
		return
	}

	orderID := string(withdrawRequest.Order)
	err := s.storageService.Withdraw(r.Context(), userData.ID, orderID, withdrawRequest.Sum)
	if err != nil {
		if err == storage.ErrNotEnoughBalance {
			http.Error(w, "", http.StatusPaymentRequired)
			return
		}
		s.logger.Error("failed to withdraw", zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *HandlersServer) apiParseRequest(r *http.Request, body interface{}) error {
	if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
		s.logger.Error("bad content type", zap.String("content_type", contentType))
		return ErrBadContentType
	}

	b, err := io.ReadAll(r.Body)
	if err != nil {
		s.logger.Error("failed to read request body", zap.Error(err))
		return err
	}

	if err = json.Unmarshal(b, &body); err != nil {
		s.logger.Error("failed to unmarshal request json", zap.Error(err))
		return ErrBodyUnmarshal
	}

	return nil
}

func (s *HandlersServer) apiWriteResponse(w http.ResponseWriter, statusCode int, response interface{}) {
	dst, err := json.Marshal(response)
	if err != nil {
		s.logger.Error("failed to marshal response", zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if _, err := w.Write(dst); err != nil {
		s.logger.Error("failed to write response body", zap.Error(err))
	}
}

func isCorrectOrderNum(number string) bool {
	digitsCount := len(number)
	isSecond := false
	sum := 0

	for i := digitsCount - 1; i >= 0; i-- {
		d := number[i] - '0'
		if isSecond {
			d = d * 2
		}

		sum += int(d) / 10
		sum += int(d) % 10

		isSecond = !isSecond
	}

	return sum%10 == 0
}
