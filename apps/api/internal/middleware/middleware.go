package middleware

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"remi-api/internal/handlers/httpx"
	"remi-api/internal/services"
)

type ctxKey string

const claimsKey ctxKey = "claims"

func ClaimsFrom(r *http.Request) *services.Claims {
	c, _ := r.Context().Value(claimsKey).(*services.Claims)
	return c
}

// Auth validates the Bearer token and stores claims on the request context.
func Auth(jwtSvc *services.JWTService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if !strings.HasPrefix(h, "Bearer ") {
				httpx.Error(w, http.StatusUnauthorized, "missing bearer token")
				return
			}
			claims, err := jwtSvc.Validate(strings.TrimPrefix(h, "Bearer "))
			if err != nil {
				httpx.Error(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}
			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

var roleRank = map[string]int{"viewer": 1, "editor": 2, "super-admin": 3}

// RequireRole allows only users whose role rank is >= min.
func RequireRole(min string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFrom(r)
			if claims == nil || roleRank[claims.Role] < roleRank[min] {
				httpx.Error(w, http.StatusForbidden, "insufficient role")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RateLimit is a simple per-IP fixed-window limiter for public form endpoints.
func RateLimit(limit int, window time.Duration) func(http.Handler) http.Handler {
	type bucket struct {
		count int
		reset time.Time
	}
	var mu sync.Mutex
	buckets := map[string]*bucket{}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr
			if host, _, err := net.SplitHostPort(ip); err == nil {
				ip = host
			}
			if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
				ip = strings.TrimSpace(strings.Split(fwd, ",")[0])
			}
			mu.Lock()
			b, ok := buckets[ip]
			now := time.Now()
			if !ok || now.After(b.reset) {
				b = &bucket{reset: now.Add(window)}
				buckets[ip] = b
			}
			b.count++
			over := b.count > limit
			mu.Unlock()

			if over {
				httpx.Error(w, http.StatusTooManyRequests, "too many requests, please try again later")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
