package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/jwtauth"
	"go.uber.org/zap"

	"github.com/real-splendid/gophermart-practicum/internal/storage"
)

const AuthCookieName = "jwt"

type userAuthRequest struct {
	Login    string
	Password string
}

type AuthServer struct {
	ctx         context.Context
	logger      *zap.Logger
	userStorage storage.AppStorage
	authorizer  *jwtauth.JWTAuth
}

func NewAuthServer(ctx context.Context, logger *zap.Logger, userStorage storage.AppStorage, authorizer *jwtauth.JWTAuth) (*AuthServer, error) {
	server := &AuthServer{
		ctx:         ctx,
		logger:      logger,
		userStorage: userStorage,
		authorizer:  authorizer,
	}

	return server, nil
}

func (s *AuthServer) registerUser(w http.ResponseWriter, r *http.Request) {
	authData := userAuthRequest{}
	if err := s.parseRequest(r, &authData); err != nil {
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	if err := s.userStorage.AddUser(r.Context(), &storage.UserAuthorization{
		Login:    authData.Login,
		Password: []byte(authData.Password),
	}); err != nil {
		if errors.Is(err, storage.ErrDuplicateUser) {
			http.Error(w, "", http.StatusConflict)
			return
		}
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	userData, err := s.userStorage.GetUserAuthInfo(r.Context(), authData.Login)
	if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	_, value, err := s.authorizer.Encode(map[string]interface{}{"id": userData.ID, "ts": time.Now().Unix()})
	if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	cookie := http.Cookie{
		Name:  AuthCookieName,
		Value: value,
		Path:  "/",
	}
	http.SetCookie(w, &cookie)

	w.WriteHeader(http.StatusOK)
}

func (s *AuthServer) login(w http.ResponseWriter, r *http.Request) {
	authData := userAuthRequest{}
	if err := s.parseRequest(r, &authData); err != nil {
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	dbUserData, err := s.userStorage.GetUserAuthInfo(r.Context(), authData.Login)
	if err != nil {
		s.logger.Error("Failed to get user info from DB", zap.Error(err))
		http.Error(w, "", http.StatusUnauthorized)
		return
	}

	if !bytes.Equal(dbUserData.Password, []byte(authData.Password)) {
		http.Error(w, "", http.StatusUnauthorized)
		return
	}

	_, value, err := s.authorizer.Encode(map[string]interface{}{"id": dbUserData.ID, "ts": time.Now().Unix()})
	if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	cookie := http.Cookie{
		Name:  AuthCookieName,
		Value: value,
		Path:  "/",
	}
	http.SetCookie(w, &cookie)

	w.WriteHeader(http.StatusOK)
}

func (s *AuthServer) parseRequest(r *http.Request, body interface{}) error {
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
