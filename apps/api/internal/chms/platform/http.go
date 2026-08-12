package platform

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"crypto/rand"
	"encoding/hex"
)

type requestIDKey struct{}

func RequestIDFrom(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey{}).(string)
	return value
}

func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if len(requestID) < 8 || len(requestID) > 128 {
			requestID = randomRequestID()
		}
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, requestID)))
	})
}

func randomRequestID() string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "request-unavailable"
	}
	return hex.EncodeToString(value)
}

func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) error {
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return ValidationError(FieldError{Path: "$", Code: "body_too_large", Message: "Request body is too large."})
		}
		return ValidationError(FieldError{Path: "$", Code: "invalid_json", Message: "Enter a valid request body."})
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return ValidationError(FieldError{Path: "$", Code: "multiple_json_values", Message: "Only one JSON object is allowed."})
	}
	return nil
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}
type errorBody struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Fields    []FieldError   `json:"fields,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
	RequestID string         `json:"requestId"`
}

func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	domain := &DomainError{Code: "internal_error", Message: "The request could not be completed."}
	var candidate *DomainError
	if errors.As(err, &candidate) {
		domain = candidate
	}
	status := map[string]int{"validation_failed": 400, "unauthenticated": 401, "forbidden": 403, "pickup_denied": 403, "not_found": 404, "version_conflict": 412, "precondition_required": 428, "idempotency_key_reused": 409, "duplicate_candidate": 409, "conflict": 409, "consent_required": 409, "invalid_transition": 409, "period_closed": 409, "attendance_locked": 409, "checkin_session_locked": 409, "reconciliation_required": 409, "rate_limited": 429, "provider_unavailable": 503, "dependency_unavailable": 503, "consent_unavailable": 503, "feature_disabled": 503}[domain.Code]
	if status == 0 {
		status = 500
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorEnvelope{Error: errorBody{Code: domain.Code, Message: domain.Message, Fields: domain.Fields, Details: domain.Details, RequestID: RequestIDFrom(r.Context())}})
}
