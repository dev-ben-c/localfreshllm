package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/rabidclock/localfreshllm/device"
)

type contextKey string

const deviceKey contextKey = "device"

// DeviceFromContext extracts the device profile from request context.
func DeviceFromContext(ctx context.Context) *device.Profile {
	p, _ := ctx.Value(deviceKey).(*device.Profile)
	return p
}

// authMiddleware validates the bearer token and injects the device profile into context.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, `{"error":"missing or invalid authorization header"}`, http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(auth, "Bearer ")
		profile, err := s.devices.GetByToken(token)
		if err != nil {
			http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), deviceKey, profile)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
