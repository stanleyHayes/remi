// Package httpx holds shared HTTP helpers for handlers.
package httpx

import (
	"encoding/json"
	"net/http"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, bson.M{"error": msg})
}

// Decode parses the JSON request body into a bson.M document.
func Decode(r *http.Request) (bson.M, error) {
	var doc bson.M
	if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
		return nil, err
	}
	return doc, nil
}

// Str returns doc[key] as a trimmed string.
func Str(doc bson.M, key string) string {
	if s, ok := doc[key].(string); ok {
		return s
	}
	return ""
}

// ObjectID parses a hex id, reporting failure with ok=false.
func ObjectID(hex string) (bson.ObjectID, bool) {
	oid, err := bson.ObjectIDFromHex(hex)
	if err != nil {
		return bson.NilObjectID, false
	}
	return oid, true
}
