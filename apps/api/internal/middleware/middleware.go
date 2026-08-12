package middleware

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"remi-api/internal/handlers/httpx"
	"remi-api/internal/services"
)

type ctxKey string

const claimsKey ctxKey = "claims"

func ClaimsFrom(r *http.Request) *services.Claims {
	c, _ := r.Context().Value(claimsKey).(*services.Claims)
	return c
}

// StaffAuth additionally checks the persisted access version for scoped staff
// tokens. Incrementing that version invalidates every existing staff token
// immediately after a role or scope change. Legacy unversioned tokens remain
// accepted only for migration and test fixtures until their natural expiry.
func StaffAuth(jwtSvc *services.JWTService, database *mongo.Database) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return Auth(jwtSvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFrom(r)
			if claims == nil || claims.Role == "member" || claims.AccessVersion == nil || database == nil {
				next.ServeHTTP(w, r)
				return
			}
			storageID := any(claims.UserID)
			if objectID, err := bson.ObjectIDFromHex(claims.UserID); err == nil {
				storageID = objectID
			}
			var row struct {
				AccessVersion    int64  `bson:"accessVersion"`
				InvitationStatus string `bson:"invitationStatus"`
				Role             string `bson:"role"`
			}
			err := database.Collection("users").FindOne(r.Context(), bson.M{"_id": storageID}).Decode(&row)
			active := row.InvitationStatus == "accepted" || row.InvitationStatus == "active" || row.InvitationStatus == ""
			if err != nil || !active || row.Role != claims.Role || row.AccessVersion != *claims.AccessVersion {
				httpx.Error(w, http.StatusUnauthorized, "access changed; sign in again")
				return
			}
			next.ServeHTTP(w, r)
		}))
	}
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

var roleRank = map[string]int{"viewer": 1, "pastor": 1, "branch-admin": 1, "membership-admin": 1, "group-admin": 1, "volunteer-coordinator": 1, "finance-counter": 1, "finance-admin": 1, "finance-approver": 1, "finance-auditor": 1, "auditor": 1, "data-protection-supervisor": 1, "editor": 2, "super-admin": 3}

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
