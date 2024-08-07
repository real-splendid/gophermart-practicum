package app

import (
	"compress/gzip"
	"context"
	"net/http"

	"github.com/go-chi/jwtauth"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/real-splendid/gophermart-practicum/internal/storage"
)

var UserAuthDataCtxKey = &contextKey{"UserAuthData"}

type gzipBodyReader struct {
	gzipReader *gzip.Reader
}

type contextKey struct {
	name string
}

func (gz *gzipBodyReader) Read(p []byte) (n int, err error) {
	return gz.gzipReader.Read(p)
}

func (gz *gzipBodyReader) Close() error {
	return gz.gzipReader.Close()
}

func DecompressGzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, "", http.StatusInternalServerError)
				return
			}
			r.Body = &gzipBodyReader{gzipReader: gz}
		}
		next.ServeHTTP(w, r)
	})
}

func AuthorizationVerifier(st storage.AppStorage, logger *zap.Logger) func(handler http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			_, claims, err := jwtauth.FromContext(ctx)
			if err != nil {
				logger.Error("failed to get claims", zap.Error(err))
				http.Error(w, "", http.StatusUnauthorized)
				return
			}

			id, exists := claims["id"]
			if !exists {
				logger.Error("failed to get user id", zap.Error(err))
				http.Error(w, "", http.StatusUnauthorized)
				return
			}

			userID, err := uuid.Parse(id.(string))
			if err != nil {
				logger.Error("failed to parse user id", zap.Error(err))
				http.Error(w, "", http.StatusUnauthorized)
				return
			}

			userData, err := st.GetUserAuthInfoByID(ctx, userID)
			if err != nil {
				logger.Error("failed to get user data", zap.Error(err))
				http.Error(w, "", http.StatusUnauthorized)
				return
			}

			ctx = context.WithValue(ctx, UserAuthDataCtxKey, userData)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func (k *contextKey) String() string {
	return "marketappauth context value " + k.name
}
