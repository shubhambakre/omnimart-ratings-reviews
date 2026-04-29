package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/walmart/ratings-reviews/internal/transport/http/response"
)

type ctxKey string

const (
	CtxRequestID ctxKey = "request_id"
	CtxCustomer  ctxKey = "customer_id"
)

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			b := make([]byte, 8)
			_, _ = rand.Read(b)
			id = hex.EncodeToString(b)
		}
		w.Header().Set("X-Request-Id", id)
		ctx := context.WithValue(r.Context(), CtxRequestID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(c int) { s.status = c; s.ResponseWriter.WriteHeader(c) }

func Logger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: 200}
			defer func() {
				log.Info("http",
					"request_id", r.Context().Value(CtxRequestID),
					"method", r.Method,
					"path", r.URL.Path,
					"status", rec.status,
					"duration_ms", time.Since(start).Milliseconds(),
				)
			}()
			next.ServeHTTP(rec, r)
		})
	}
}

func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("panic", "err", rec, "stack", string(debug.Stack()))
					response.Err(w, http.StatusInternalServerError, "INTERNAL", "internal error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// CustomerAuth is a stub. In production this would verify a JWT issued by the
// shopper identity service and assert scopes; here we trust X-Customer-Id.
func CustomerAuth(required bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cid := r.Header.Get("X-Customer-Id")
			if required && cid == "" {
				response.Err(w, http.StatusUnauthorized, "UNAUTHORIZED", "X-Customer-Id required")
				return
			}
			ctx := context.WithValue(r.Context(), CtxCustomer, cid)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// APIKeyAuth gates the internal/non-site-facing API. In production this would
// be replaced by mTLS + a service-mesh-issued SPIFFE identity.
func APIKeyAuth(expected string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if expected == "" || r.Header.Get("X-Api-Key") != expected {
				response.Err(w, http.StatusUnauthorized, "UNAUTHORIZED", "valid X-Api-Key required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func Chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}
